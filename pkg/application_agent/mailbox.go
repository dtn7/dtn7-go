// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package application_agent

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/store"
)

// Mailbox provides message storage/querying/delivery for application agents to use.
// The Mailbox does not store the actual bundles so we don't have to keep all bundles in memory all the time.
// The boolean values of the messages-map signify whether the bundle has already been retrieved
type Mailbox struct {
	rwMutex sync.RWMutex

	messages map[bpv7.BundleID]bool
}

func NewMailbox() *Mailbox {
	mailbox := Mailbox{
		messages: make(map[bpv7.BundleID]bool),
	}
	return &mailbox
}

// Deliver delivers bundle to mailbox.
// Bundle has to have been stored in the store before delivery
// Returns AlreadyDeliveredError or error from the store.
// Returns AlreadyDeliveredError if bundle with same BundleID is already stored.
func (mailbox *Mailbox) Deliver(bndl *store.BundleDescriptor) error {
	log.WithField("bid", bndl.ID).Debug("Delivering bundle to mailbox")

	mailbox.rwMutex.Lock()
	defer mailbox.rwMutex.Unlock()

	bid := bndl.ID

	if _, ok := mailbox.messages[bid]; ok {
		return NewAlreadyDeliveredError(bid)
	}

	if _, err := store.GetStoreSingleton().LoadBundleDescriptor(bndl.ID); err != nil {
		return err
	}

	mailbox.messages[bid] = false

	return nil
}

// List returns a list of the BundleIDs of all bundles stored in the mailbox.
func (mailbox *Mailbox) List() []bpv7.BundleID {
	mailbox.rwMutex.RLock()
	defer mailbox.rwMutex.RUnlock()

	bundles := make([]bpv7.BundleID, 0, len(mailbox.messages))
	for bndl := range mailbox.messages {
		bundles = append(bundles, bndl)
	}

	return bundles
}

// ListNew returns a list of the BundleIDs of all bundles stored in the mailbox which have not been retrieved before.
func (mailbox *Mailbox) ListNew() []bpv7.BundleID {
	mailbox.rwMutex.RLock()
	defer mailbox.rwMutex.RUnlock()

	bundles := make([]bpv7.BundleID, 0, len(mailbox.messages))
	for bid := range mailbox.messages {
		if !mailbox.messages[bid] {
			bundles = append(bundles, bid)
		}
	}

	return bundles
}

// Get returns the Bundle for a given BundleID.
// If remove is set, then the bundle will be deleted from the mailbox
// Returns NewNoSuchBundleError or error coming from the store.
// Returns NewNoSuchBundleError if no bundle for the given ID is stored
func (mailbox *Mailbox) Get(bid bpv7.BundleID, remove bool) (*bpv7.Bundle, error) {
	if remove {
		mailbox.rwMutex.Lock()
		defer mailbox.rwMutex.Unlock()
	} else {
		mailbox.rwMutex.RLock()
		defer mailbox.rwMutex.RUnlock()
	}

	_, ok := mailbox.messages[bid]
	if !ok {
		return nil, NewNoSuchBundleError(bid)
	}

	bd, err := store.GetStoreSingleton().LoadBundleDescriptor(bid)
	if err != nil {
		return nil, err
	}

	bndl, err := bd.Load()
	if err != nil {
		return nil, err
	}

	if remove {
		delete(mailbox.messages, bid)
	} else {
		mailbox.messages[bid] = true
	}

	return bndl, nil
}

// GetAll returns slice of all bundles in the mailbox.
// If remove is set, then the mailbox will be cleared.
// Returns error coming from the store.
// In case of an error, still returns all bundles that were successfully loaded before the error
func (mailbox *Mailbox) GetAll(remove bool) ([]*bpv7.Bundle, error) {
	if remove {
		mailbox.rwMutex.Lock()
		defer mailbox.rwMutex.Unlock()
	} else {
		mailbox.rwMutex.RLock()
		defer mailbox.rwMutex.RUnlock()
	}

	bndls := make([]*bpv7.Bundle, 0, len(mailbox.messages))
	for bid := range mailbox.messages {
		bd, err := store.GetStoreSingleton().LoadBundleDescriptor(bid)
		if err != nil {
			return nil, err
		}

		bndl, err := bd.Load()
		if err != nil {
			return bndls, err
		}

		bndls = append(bndls, bndl)
		mailbox.messages[bid] = true
	}

	if remove {
		clear(mailbox.messages)
	}

	return bndls, nil
}

// GetNew returns slice of all bundles in the mailbox that have not been retrieved before.
// If remove is set, then the mailbox will be cleared.
// Returns error coming from the store.
// In case of an error, still returns all bundles that were successfully loaded before the error
func (mailbox *Mailbox) GetNew(remove bool) ([]*bpv7.Bundle, error) {
	if remove {
		mailbox.rwMutex.Lock()
		defer mailbox.rwMutex.Unlock()
	} else {
		mailbox.rwMutex.RLock()
		defer mailbox.rwMutex.RUnlock()
	}

	bndls := make([]*bpv7.Bundle, 0, len(mailbox.messages))
	for bid := range mailbox.messages {
		if mailbox.messages[bid] {
			continue
		}

		bd, err := store.GetStoreSingleton().LoadBundleDescriptor(bid)
		if err != nil {
			return nil, err
		}

		bndl, err := bd.Load()
		if err != nil {
			return bndls, err
		}

		bndls = append(bndls, bndl)
		mailbox.messages[bid] = true
	}

	if remove {
		clear(mailbox.messages)
	}

	return bndls, nil
}

func (mailbox *Mailbox) Delete(bid bpv7.BundleID) {
	mailbox.rwMutex.Lock()
	defer mailbox.rwMutex.Unlock()

	delete(mailbox.messages, bid)
}

func (mailbox *Mailbox) Clear() {
	mailbox.rwMutex.Lock()
	defer mailbox.rwMutex.Unlock()

	clear(mailbox.messages)
}

// GC runs garbage collection on mailbox
// Removes all bundles which can longer be loaded from the store (most likely because they have been deleted)
func (mailbox *Mailbox) GC() {
	mailbox.rwMutex.Lock()
	defer mailbox.rwMutex.Unlock()

	for bid := range mailbox.messages {
		_, err := store.GetStoreSingleton().LoadBundleDescriptor(bid)
		if err != nil {
			delete(mailbox.messages, bid)
		}
	}
}
