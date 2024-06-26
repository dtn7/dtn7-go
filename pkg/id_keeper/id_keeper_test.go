// SPDX-FileCopyrightText: 2019, 2020 Alvar Penning
// SPDX-FileCopyrightText: 2023, 2024 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package id_keeper

import (
	"testing"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

func TestIdKeeper(t *testing.T) {
	bndl0, err := bpv7.Builder().
		Source("dtn://src/").
		Destination("dtn://dest/").
		CreationTimestampEpoch().
		Lifetime("60s").
		BundleCtrlFlags(bpv7.MustNotFragmented|bpv7.RequestStatusTime).
		BundleAgeBlock(0, bpv7.DeleteBundle).
		PayloadBlock([]byte("hello world!")).
		Build()
	if err != nil {
		t.Errorf("Creating bundle failed: %v", err)
	}

	bndl1, err := bpv7.Builder().
		Source("dtn://src/").
		Destination("dtn://dest/").
		CreationTimestampEpoch().
		Lifetime("60s").
		BundleCtrlFlags(bpv7.MustNotFragmented|bpv7.RequestStatusTime).
		BundleAgeBlock(0, bpv7.DeleteBundle).
		PayloadBlock([]byte("hello world!")).
		Build()
	if err != nil {
		t.Errorf("Creating bundle failed: %v", err)
	}

	_ = InitializeIdKeeper()

	keeper := GetIdKeeperSingleton()

	keeper.Update(&bndl0)
	keeper.Update(&bndl1)

	if seq := bndl0.PrimaryBlock.CreationTimestamp.SequenceNumber(); seq != 0 {
		t.Errorf("First bundle's sequence number is %d", seq)
	}

	if seq := bndl1.PrimaryBlock.CreationTimestamp.SequenceNumber(); seq != 1 {
		t.Errorf("Second bundle's sequence number is %d", seq)
	}
}
