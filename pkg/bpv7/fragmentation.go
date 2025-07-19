// SPDX-FileCopyrightText: 2019, 2020, 2021 Alvar Penning
// SPDX-FileCopyrightText: 2022 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package bpv7

import (
	"bytes"
	"fmt"
	"math"
	"sort"

	"github.com/dtn7/cboring"
)

// Fragment a Bundle into multiple Bundles, with each serialized Bundle limited to mtu bytes.
// TODO: rewrite this to better account for a separate PayloadBlock
func (b *Bundle) Fragment(mtu int) (bs []*Bundle, err error) {
	if b.PrimaryBlock.BundleControlFlags.Has(MustNotFragmented) {
		err = fmt.Errorf("bundle control flags forbids bundle fragmentation")
		return
	}

	var (
		cborOverhead     = 2
		extFirstOverhead int
		extOtherOverhead int

		payloadBlock    *CanonicalBlock
		payloadBlockLen int
	)

	payloadBlock = &b.PayloadBlock
	payloadBlockLen = len(b.PayloadBlock.Value.(*PayloadBlock).Data())

	if extFirstOverhead, extOtherOverhead, err = fragmentExtensionBlocksLen(b, mtu); err != nil {
		return
	}

	for i := 0; i < payloadBlockLen; {
		var (
			fragPrimaryBlock PrimaryBlock
			primaryOverhead  int
		)

		if fragPrimaryBlock, primaryOverhead, err = fragmentPrimaryBlock(b.PrimaryBlock, i, payloadBlockLen); err != nil {
			return
		}

		overhead := cborOverhead + primaryOverhead
		if i == 0 {
			overhead += extFirstOverhead
		} else {
			overhead += extOtherOverhead
		}

		if overhead >= mtu {
			err = fmt.Errorf("bundle overhead of fragment %d exceeds MTU", i)
			return
		}

		fragBundle := MustNewBundle(fragPrimaryBlock, nil, CanonicalBlock{})

		for _, cb := range b.ExtensionBlocks {
			if i > 0 && !cb.BlockControlFlags.Has(ReplicateBlock) {
				continue
			}

			if err = fragBundle.AddExtensionBlock(cb); err != nil {
				return
			}
		}

		fragPayloadBlockLen := mtu - overhead

		offset := int(math.Min(float64(i+fragPayloadBlockLen), float64(len(payloadBlock.Value.(*PayloadBlock).Data()))))
		fragBundle.PayloadBlock = CanonicalBlock{
			BlockNumber:       1,
			BlockControlFlags: payloadBlock.BlockControlFlags,
			CRCType:           payloadBlock.CRCType,
			Value:             NewPayloadBlock(payloadBlock.Value.(*PayloadBlock).Data()[i:offset])}

		if err = fragBundle.CheckValid(); err != nil {
			return
		}
		bs = append(bs, fragBundle)

		i += fragPayloadBlockLen
	}

	if len(bs) == 1 {
		bs = []*Bundle{b}
	}

	return
}

// fragmentPrimaryBlock creates a fragment's Primary Block and calculates its length.
func fragmentPrimaryBlock(pb PrimaryBlock, fragmentOffset, totalDataLength int) (fragPb PrimaryBlock, l int, err error) {
	fragPb = PrimaryBlock{
		Version:            pb.Version,
		BundleControlFlags: pb.BundleControlFlags | IsFragment,
		CRCType:            pb.CRCType,
		Destination:        pb.Destination,
		SourceNode:         pb.SourceNode,
		ReportTo:           pb.ReportTo,
		CreationTimestamp:  pb.CreationTimestamp,
		Lifetime:           pb.Lifetime,
		FragmentOffset:     uint64(fragmentOffset),
		TotalDataLength:    uint64(totalDataLength),
	}

	buff := new(bytes.Buffer)

	err = fragPb.MarshalCbor(buff)
	l = buff.Len()
	return
}

// fragmentExtensionBlocksLen calculates the estimated maximum length for the Extension Blocks for the
// first and the other fragments.
func fragmentExtensionBlocksLen(b *Bundle, mtu int) (first int, others int, err error) {
	buff := new(bytes.Buffer)

	for _, eb := range b.ExtensionBlocks {
		if eb.TypeCode() == BlockTypePayloadBlock {
			eb = CanonicalBlock{
				BlockNumber:       eb.BlockNumber,
				BlockControlFlags: eb.BlockControlFlags,
				Value:             NewPayloadBlock(nil),
			}
		}

		eb.CRCType = CRC32

		if err = eb.MarshalCbor(buff); err != nil {
			return
		}

		cbLen := buff.Len()
		first += cbLen
		if eb.BlockControlFlags.Has(ReplicateBlock) {
			others += cbLen
		}

		if eb.TypeCode() == BlockTypePayloadBlock {
			// Update the byte string length field
			buff.Reset()
			if err = cboring.WriteByteStringLen(uint64(mtu), buff); err != nil {
				return
			}
			first += buff.Len() - 1
			others += cbLen + buff.Len() - 1
		}

		buff.Reset()
	}

	eb := CanonicalBlock{
		BlockNumber:       1,
		BlockControlFlags: b.PayloadBlock.BlockControlFlags,
		Value:             NewPayloadBlock(nil),
	}

	eb.CRCType = CRC32

	if err = eb.MarshalCbor(buff); err != nil {
		return
	}

	cbLen := buff.Len()
	first += cbLen

	buff.Reset()
	if err = cboring.WriteByteStringLen(uint64(mtu), buff); err != nil {
		return
	}
	first += buff.Len() - 1
	others += cbLen + buff.Len() - 1

	return
}

// prepareReassembly sorts the slice of Bundle fragments and checks if their are any gaps left.
func prepareReassembly(bs []*Bundle) error {
	if len(bs) == 0 {
		return fmt.Errorf("slice of fragments is empty")
	}

	sort.Slice(bs, func(i, j int) bool {
		return bs[i].PrimaryBlock.FragmentOffset < bs[j].PrimaryBlock.FragmentOffset
	})

	lastIndex := uint64(0)
	for _, b := range bs {
		if !b.PrimaryBlock.BundleControlFlags.Has(IsFragment) {
			return fmt.Errorf("bundle is not a fragment")
		}

		if fragOff := b.PrimaryBlock.FragmentOffset; fragOff > lastIndex {
			return fmt.Errorf("next fragment starts at offset %d, gap from %d to %d", fragOff, lastIndex, fragOff)
		} else {
			lastIndex = fragOff + uint64(len(b.PayloadBlock.Value.(*PayloadBlock).Data()))
		}
	}

	if total := bs[0].PrimaryBlock.TotalDataLength; total != lastIndex {
		return fmt.Errorf("last index is %d and does not match total length of %d", lastIndex, total)
	}

	return nil
}

// IsBundleReassemblable checks if a Bundle can be reassembled from the given fragments. This method might sort the
// given array as a side effect.
func IsBundleReassemblable(bs []*Bundle) bool {
	return prepareReassembly(bs) == nil
}

// mergeFragmentPayload merges the fragmented payload.
func mergeFragmentPayload(bs []*Bundle) (data []byte, err error) {
	lastIndex := 0
	for _, b := range bs {
		var (
			fragStartIndex  int
			fragPayloadData []byte
		)

		fragStartIndex = int(b.PrimaryBlock.FragmentOffset)
		fragPayloadData = b.PayloadBlock.Value.(*PayloadBlock).Data()

		data = append(data, fragPayloadData[lastIndex-fragStartIndex:]...)
		lastIndex = fragStartIndex + len(fragPayloadData)
	}

	return
}

// ReassembleFragments merges a slice of Bundle fragments into the reassembled Bundle.
func ReassembleFragments(bs []*Bundle) (b *Bundle, err error) {
	b = &Bundle{}
	if err = prepareReassembly(bs); err != nil {
		return
	}

	b.PrimaryBlock = bs[0].PrimaryBlock
	b.PrimaryBlock.BundleControlFlags &^= IsFragment
	b.PrimaryBlock.FragmentOffset = 0
	b.PrimaryBlock.TotalDataLength = 0
	b.PrimaryBlock.CRC = nil

	for _, cb := range bs[0].ExtensionBlocks {
		if err = b.AddExtensionBlock(cb); err != nil {
			return
		}
	}

	if payload, payloadErr := mergeFragmentPayload(bs); payloadErr != nil {
		err = payloadErr
		return
	} else {
		pb0 := bs[0].PayloadBlock

		cb := NewCanonicalBlock(1, pb0.BlockControlFlags, NewPayloadBlock(payload))
		cb.SetCRCType(pb0.CRCType)

		b.PayloadBlock = cb
	}

	err = b.CheckValid()
	return
}
