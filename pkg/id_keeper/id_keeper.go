// SPDX-FileCopyrightText: 2019, 2020 Alvar Penning
// SPDX-FileCopyrightText: 2024 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package id_keeper provides the IdKeeper.
// A (non-fragment) bundle's id consists of 2 parts:
// 1. The sending node's EndpointID
// 2. A two-part `timestamp`
//
// The timestamp's first part is the time of create (in millisecond precision),
// while the second part is a counter which allows for multiple bundles to be created in the same millisecond.
//
// The IdKeeper is responsible for tracking the setting the id of newly-created bundle and ensuring that they are unique.
//
// Since there should only be a single IdKeeper active at any time, this package employs the singleton pattern.
// Use `InitializeIdKeeper` and `GetIdKeeperSingleton.`
package id_keeper

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

// idTuple is a tuple struct for looking up a bundle's ID - based on its source
// node and DTN time part of the creation timestamp.
type idTuple struct {
	source bpv7.EndpointID
	time   bpv7.DtnTime
}

// newIdTuple creates an idTuple based on the given bundle.
func newIdTuple(bndl *bpv7.Bundle) idTuple {
	return idTuple{
		source: bndl.PrimaryBlock.SourceNode,
		time:   bndl.PrimaryBlock.CreationTimestamp.DtnTime(),
	}
}

var idKeeperSingleton *IdKeeper

// IdKeeper keeps track of the creation timestamp's sequence number for
// outgoing bundles.
type IdKeeper struct {
	data  map[idTuple]uint64
	mutex sync.Mutex
}

func InitializeIdKeeper() {
	if idKeeperSingleton != nil {
		log.Fatalf("Attempting to access an uninitialised IdKeeper. This must never happen!")
	}

	idKeeperSingleton = &IdKeeper{
		data: make(map[idTuple]uint64),
	}
}

func GetIdKeeperSingleton() *IdKeeper {
	if idKeeperSingleton == nil {
		log.Fatalf("Attempting to access an uninitialised IdKeeper. This must never happen!")
	}
	return idKeeperSingleton
}

// Update updates the IdKeeper's state regarding this bundle and sets this
// bundle's sequence number.
func (idk *IdKeeper) Update(bndl *bpv7.Bundle) {
	var tpl = newIdTuple(bndl)

	idk.mutex.Lock()
	defer idk.mutex.Unlock()
	if state, ok := idk.data[tpl]; ok {
		idk.data[tpl] = state + 1
	} else {
		idk.data[tpl] = 0
	}

	bndl.PrimaryBlock.CreationTimestamp[1] = idk.data[tpl]
}

// Clean removes states which are older an hour and aren't the epoch time.
func (idk *IdKeeper) Clean() {
	idk.mutex.Lock()
	defer idk.mutex.Unlock()

	var threshold = bpv7.DtnTimeFromTime(time.Now().Add(-time.Hour))

	for tpl := range idk.data {
		if tpl.time < threshold {
			delete(idk.data, tpl)
		}
	}
}
