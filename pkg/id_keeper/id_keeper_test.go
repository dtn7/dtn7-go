// SPDX-FileCopyrightText: 2019, 2020 Alvar Penning
// SPDX-FileCopyrightText: 2023, 2024 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package id_keeper

import (
	"testing"

	"github.com/dtn7/dtn7-go/internal/testhelpers"
)

func TestIdKeeper(t *testing.T) {
	bndl0 := testhelpers.SampleBundle(t)
	bndl1 := testhelpers.SampleBundle(t)

	InitializeIdKeeper()

	keeper := GetIdKeeperSingleton()

	keeper.Update(bndl0)
	keeper.Update(bndl1)

	if seq := bndl0.PrimaryBlock.CreationTimestamp.SequenceNumber(); seq != 0 {
		t.Errorf("First bundle's sequence number is %d", seq)
	}

	if seq := bndl1.PrimaryBlock.CreationTimestamp.SequenceNumber(); seq != 1 {
		t.Errorf("Second bundle's sequence number is %d", seq)
	}
}
