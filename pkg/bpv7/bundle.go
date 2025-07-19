// SPDX-FileCopyrightText: 2018, 2019, 2020, 2022 Alvar Penning
// SPDX-FileCopyrightText: 2022, 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package bpv7

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/dtn7/cboring"
	"github.com/hashicorp/go-multierror"
)

// Bundle represents a bundle as defined in section 4.2.1. Each Bundle contains
// one primary block and multiple canonical blocks.
type Bundle struct {
	PrimaryBlock    PrimaryBlock
	ExtensionBlocks []CanonicalBlock
	PayloadBlock    CanonicalBlock
}

// NewBundle creates a new Bundle. The values and flags of the blocks will be
// checked and an error might be returned.
func NewBundle(primary PrimaryBlock, extensions []CanonicalBlock, payload CanonicalBlock) (b *Bundle, err error) {
	b = MustNewBundle(primary, extensions, payload)
	err = b.CheckValid()

	return
}

// MustNewBundle creates a new Bundle like NewBundle, but skips the validity
// check. No panic will be called!
func MustNewBundle(primary PrimaryBlock, extensions []CanonicalBlock, payload CanonicalBlock) *Bundle {
	b := Bundle{
		PrimaryBlock:    primary,
		ExtensionBlocks: extensions,
		PayloadBlock:    payload,
	}
	b.sortExtensionBlocks()

	return &b
}

// ParseBundle reads a new CBOR encoded Bundle from a Reader.
func ParseBundle(r io.Reader) (b *Bundle, err error) {
	b = &Bundle{}
	err = cboring.Unmarshal(b, r)
	return
}

// WriteBundle writes this Bundle CBOR encoded into a Writer.
func (b *Bundle) WriteBundle(w io.Writer) error {
	return cboring.Marshal(b, w)
}

// forEachBlock applies the given function for each of this Bundle's blocks.
func (b *Bundle) forEachBlock(f func(block)) {
	f(&b.PrimaryBlock)
	for i := 0; i < len(b.ExtensionBlocks); i++ {
		f(&b.ExtensionBlocks[i])
	}
	f(&b.PayloadBlock)
}

// ExtensionBlocksByType returns all this Bundle's canonical block/extension blocks
// matching the requested block type code. If no such block was found,
// an error will be returned.
func (b *Bundle) ExtensionBlocksByType(blockType uint64) (cbs []*CanonicalBlock, err error) {
	for i := 0; i < len(b.ExtensionBlocks); i++ {
		cb := &b.ExtensionBlocks[i]
		if cb.TypeCode() == blockType {
			cbs = append(cbs, cb)
		}
	}

	if len(cbs) == 0 {
		cbs = nil
		err = fmt.Errorf("no CanonicalBlock with block type %d was found in Bundle", blockType)
	}
	return
}

// ExtensionBlockByType returns a Canonical Block for the requested type code.
// If there is no such Block or more than exactly one Block, an error will be returned.
func (b *Bundle) ExtensionBlockByType(blockType uint64) (*CanonicalBlock, error) {
	cbs, err := b.ExtensionBlocksByType(blockType)

	if err != nil {
		return nil, err
	} else if l := len(cbs); l != 1 {
		return nil, fmt.Errorf("there are %d Extension Blocks for type code %d", l, blockType)
	} else {
		return cbs[0], nil
	}
}

// HasExtensionBlock checks if a ExtensionBlock for some block type number is present.
func (b *Bundle) HasExtensionBlock(blockType uint64) bool {
	_, err := b.ExtensionBlocksByType(blockType)
	return err == nil
}

// sortExtensionBlocks sorts the extension blocks.
// This method is called internally after block modification, e.g., in MustNewBundle or Bundle.AddExtensionBlock.
func (b *Bundle) sortExtensionBlocks() {
	sort.Sort(canonicalBlockNumberSort(b.ExtensionBlocks))
}

// AddExtensionBlock adds a new ExtensionBlock to this Bundle.
// The block number will be calculated and overwritten within this method.
func (b *Bundle) AddExtensionBlock(block CanonicalBlock) error {
	// TODO: return error if we try to add a block which already exists
	var blockNumbers []uint64
	for i := 0; i < len(b.ExtensionBlocks); i++ {
		blockNumbers = append(blockNumbers, b.ExtensionBlocks[i].BlockNumber)
	}

	var blockNumber uint64 = 1
	if block.Value.BlockTypeCode() != BlockTypePayloadBlock {
		blockNumber = 2
	}

	for {
		flag := true
		for _, no := range blockNumbers {
			if blockNumber == no {
				flag = false
				break
			}
		}

		if flag {
			break
		} else {
			blockNumber += 1
		}
	}

	block.BlockNumber = blockNumber

	b.ExtensionBlocks = append(b.ExtensionBlocks, block)
	b.sortExtensionBlocks()
	return nil
}

// GetExtensionBlockByBlockNumber  searches and returns a CanonicalBlock / ExtensionBlock with the given block number.
// If no such block exists, the method will return an error. Sorting will not be performed, as we assume that the blocks are
// already in their correct order.
func (b *Bundle) GetExtensionBlockByBlockNumber(blockNumber uint64) (blockFound *CanonicalBlock, err error) {
	for i := 0; i < len(b.ExtensionBlocks); i++ {
		if b.ExtensionBlocks[i].BlockNumber == blockNumber {
			return &b.ExtensionBlocks[i], nil
		}
	}
	return nil, fmt.Errorf("block with number %d not found", blockNumber)
}

// RemoveExtensionBlockByBlockNumber searches and removes a CanonicalBlock / ExtensionBlock with the given block number.
//
// If no such block exists, the method will do nothing. Sorting will not be performed, as we assume that the blocks are
// already in their correct order.
func (b *Bundle) RemoveExtensionBlockByBlockNumber(blockNumber uint64) {
	for i := 0; i < len(b.ExtensionBlocks); i++ {
		if b.ExtensionBlocks[i].BlockNumber == blockNumber {
			b.ExtensionBlocks = append(b.ExtensionBlocks[:i], b.ExtensionBlocks[i+1:]...)
			return
		}
	}
}

// SetCRCType sets the given CRCType for each block. To also calculate and set
// the CRC value, one should also call the CalculateCRC method.
func (b *Bundle) SetCRCType(crcType CRCType) {
	b.forEachBlock(func(blck block) {
		blck.SetCRCType(crcType)
	})
}

// ID returns a BundleID representing this Bundle.
func (b *Bundle) ID() BundleID {
	return BundleID{
		SourceNode: b.PrimaryBlock.SourceNode,
		Timestamp:  b.PrimaryBlock.CreationTimestamp,

		IsFragment:      b.PrimaryBlock.BundleControlFlags.Has(IsFragment),
		FragmentOffset:  b.PrimaryBlock.FragmentOffset,
		TotalDataLength: b.PrimaryBlock.TotalDataLength,
	}
}

func (b *Bundle) String() string {
	return b.ID().String()
}

// IsLifetimeExceeded of this Bundle by checking an optional Bundle Age Block and the PrimaryBlock's Lifetime.
func (b *Bundle) IsLifetimeExceeded() bool {
	if b.PrimaryBlock.CreationTimestamp.IsZeroTime() {
		if bab, err := b.ExtensionBlockByType(BlockTypeBundleAgeBlock); err != nil {
			return true
		} else {
			return bab.Value.(*BundleAgeBlock).Age() > b.PrimaryBlock.Lifetime
		}
	}

	maxTimestamp := b.PrimaryBlock.CreationTimestamp.DtnTime().Time().Add(
		time.Duration(b.PrimaryBlock.Lifetime) * time.Millisecond)
	return time.Now().After(maxTimestamp)
}

// CheckValid returns an array of errors for incorrect data.
func (b *Bundle) CheckValid() (errs error) {
	// Check blocks for errors
	b.forEachBlock(func(blck block) {
		if blckErr := blck.CheckValid(); blckErr != nil {
			errs = multierror.Append(errs, blckErr)
		}
	})

	if b.PayloadBlock.BlockControlFlags.Has(StatusReportBlock) {
		errs = multierror.Append(errs,
			fmt.Errorf("bundle: bundle processing control flags indicate that "+
				"this bundle's payload is an administrative record or the source "+
				"node is omitted, but the \"transmit status report if block "+
				"cannot be processed\" block processing control flag was set in a "+
				"canonical block"))
	}

	// Check uniqueness of block numbers
	var cbBlockNumbers = make(map[uint64]bool)

	for _, cb := range b.ExtensionBlocks {
		// Check block numbers
		if _, ok := cbBlockNumbers[cb.BlockNumber]; ok {
			errs = multierror.Append(errs,
				fmt.Errorf("bundle: block number %d occurred multiple times", cb.BlockNumber))
		}
		cbBlockNumbers[cb.BlockNumber] = true

		// Context aware block self-check
		if blckErr := cb.Value.CheckContextValid(b); blckErr != nil {
			errs = multierror.Append(errs, blckErr)
		}
	}

	// Check existence of a Bundle Age Block if the CreationTimestamp is zero.
	if b.PrimaryBlock.CreationTimestamp.IsZeroTime() {
		if !b.HasExtensionBlock(BlockTypeBundleAgeBlock) {
			errs = multierror.Append(errs, fmt.Errorf(
				"Bundle: Creation Timestamp is zero, but no Bundle Age block exists"))
		}
	}

	// Check if the Bundle's lifetime is exceeded
	if b.IsLifetimeExceeded() {
		errs = multierror.Append(errs, fmt.Errorf("Bundle: Lifetime is exceeded"))
	}

	return
}

// IsAdministrativeRecord returns if this Bundle's control flags indicate this
// has an administrative record payload.
func (b *Bundle) IsAdministrativeRecord() bool {
	return b.PrimaryBlock.BundleControlFlags.Has(AdministrativeRecordPayload)
}

// AdministrativeRecord stored within this Bundle.
// An error arises if this Bundle is not an AdministrativeRecord, compare IsAdministrativeRecord.
func (b *Bundle) AdministrativeRecord() (AdministrativeRecord, error) {
	if !b.IsAdministrativeRecord() {
		return nil, fmt.Errorf("bundle is not an administrative record")
	}

	buff := bytes.NewBuffer(b.PayloadBlock.Value.(*PayloadBlock).Data())
	return GetAdministrativeRecordManager().ReadAdministrativeRecord(buff)
}

// MarshalCbor writes this Bundle's CBOR representation.
func (b *Bundle) MarshalCbor(w io.Writer) error {
	if _, err := w.Write([]byte{cboring.IndefiniteArray}); err != nil {
		return err
	}

	if err := cboring.Marshal(&b.PrimaryBlock, w); err != nil {
		return fmt.Errorf("PrimaryBlock failed: %v", err)
	}

	for i := 0; i < len(b.ExtensionBlocks); i++ {
		if err := cboring.Marshal(&b.ExtensionBlocks[i], w); err != nil {
			return fmt.Errorf("ExtensionBlock failed: %v", err)
		}
	}

	if err := cboring.Marshal(&b.PayloadBlock, w); err != nil {
		return fmt.Errorf("PayloadBlock failed: %v", err)
	}

	if _, err := w.Write([]byte{cboring.BreakCode}); err != nil {
		return err
	}

	return nil
}

// UnmarshalCbor creates this Bundle based on a CBOR representation.
func (b *Bundle) UnmarshalCbor(r io.Reader) error {
	if err := cboring.ReadExpect(cboring.IndefiniteArray, r); err != nil {
		return err
	}

	if err := cboring.Unmarshal(&b.PrimaryBlock, r); err != nil {
		return fmt.Errorf("PrimaryBlock failed: %v", err)
	}

	if b.ExtensionBlocks == nil {
		b.ExtensionBlocks = make([]CanonicalBlock, 0)
	}
	for {
		cb := CanonicalBlock{}
		//TODO: use new error handling
		if err := cboring.Unmarshal(&cb, r); err == cboring.FlagBreakCode {
			break
		} else if err != nil {
			return fmt.Errorf("CanonicalBlock failed: %v", err)
		} else {
			if cb.TypeCode() == BlockTypePayloadBlock {
				b.PayloadBlock = cb
			} else {
				b.ExtensionBlocks = append(b.ExtensionBlocks, cb)
			}
		}
	}

	return b.CheckValid()
}

// MarshalJSON creates a JSON object for this Bundle.
func (b *Bundle) MarshalJSON() ([]byte, error) {
	extensions := make([]json.Marshaler, len(b.ExtensionBlocks))
	for i := range b.ExtensionBlocks {
		extensions[i] = b.ExtensionBlocks[i]
	}

	return json.Marshal(&struct {
		PrimaryBlock    json.Marshaler   `json:"primaryBlock"`
		ExtensionBlocks []json.Marshaler `json:"canonicalBlocks"`
		PayloadBlock    json.Marshaler   `json:"payloadBlock"`
	}{
		PrimaryBlock:    b.PrimaryBlock,
		ExtensionBlocks: extensions,
		PayloadBlock:    b.PayloadBlock,
	})
}
