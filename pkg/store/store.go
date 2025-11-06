// SPDX-FileCopyrightText: 2023, 2024, 2025 Markus Sommer
// SPDX-FileCopyrightText: 2023, 2024 Artur Sterz
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package store implements on-disk persistence for bundle's and their metadata
// Uses Badgerhold (github.com/timshannon/badgerhold) for persisting metadata.
// Bundles are stored in CBOR-serialized form on-disk.
//
// Since there should only be a single BundleStore active at any time, this package employs the singleton pattern.
// Use `InitialiseStore` and `GetStoreSingleton.`
package store

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/timshannon/badgerhold/v4"

	"github.com/dtn7/cboring"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

type BundleStore struct {
	nodeID          bpv7.EndpointID
	metadataStore   *badgerhold.Store
	bundleDirectory string
}

var storeSingleton *BundleStore

// InitialiseStore initialises the store singleton
// To access Singleton-instance, use GetStoreSingleton
// Further calls to this function after initialisation will panic.
func InitialiseStore(nodeID bpv7.EndpointID, path string) error {
	if storeSingleton != nil {
		log.Fatalf("Attempting to access an uninitialised store. This must never happen!")
	}

	opts := badgerhold.DefaultOptions
	opts.Dir = path
	opts.ValueDir = path

	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}

	badgerStore, err := badgerhold.Open(opts)
	if err != nil {
		return err
	}

	bundleDirectory := filepath.Join(path, "bundles")
	if err := os.MkdirAll(bundleDirectory, 0700); err != nil {
		return err
	}

	storeSingleton = &BundleStore{nodeID: nodeID, metadataStore: badgerStore, bundleDirectory: bundleDirectory}

	return nil
}

// GetStoreSingleton returns the store singleton-instance.
// Attempting to call this function before store initialisation will cause the program to panic.
func GetStoreSingleton() *BundleStore {
	if storeSingleton == nil {
		log.Fatal("Attempting to access an uninitialised store. This must never happen!")
	}
	return storeSingleton
}

func (bst *BundleStore) Shutdown() error {
	storeSingleton = nil
	err := bst.metadataStore.Close()
	return err
}

func (bst *BundleStore) LoadBundleDescriptor(bundleId bpv7.BundleID) (*BundleDescriptor, error) {
	idString := bundleId.String()
	bd := BundleDescriptor{}
	err := bst.metadataStore.Get(idString, &bd)
	return &bd, err
}

func (bst *BundleStore) GetWithConstraint(constraint Constraint) ([]*BundleDescriptor, error) {
	bundles := make([]BundleDescriptor, 0)
	err := bst.metadataStore.Find(&bundles, badgerhold.Where("RetentionConstraints").Contains(constraint))
	if err != nil {
		return nil, err
	}

	ptrs := make([]*BundleDescriptor, len(bundles))
	for i, bndl := range bundles {
		ptrs[i] = &bndl
	}

	return ptrs, nil
}

func (bst *BundleStore) GetDispatchable() ([]*BundleDescriptor, error) {
	bundles := make([]BundleDescriptor, 0)
	err := bst.metadataStore.Find(&bundles, badgerhold.Where("Dispatch").Eq(true))
	if err != nil {
		return nil, err
	}

	ptrs := make([]*BundleDescriptor, len(bundles))
	for i := range bundles {
		ptrs[i] = &bundles[i]
	}

	return ptrs, nil
}

func (bst *BundleStore) loadEntireBundle(filename string) (*bpv7.Bundle, error) {
	path := filepath.Join(bst.bundleDirectory, filename)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bundle, err := bpv7.ParseBundle(f)

	return bundle, nil
}

func (bst *BundleStore) insertNewBundle(bundle *bpv7.Bundle) (*BundleDescriptor, error) {
	log.WithField("bundle", bundle.ID().String()).Debug("Inserting new bundle")
	lifetimeDuration := time.Millisecond * time.Duration(bundle.PrimaryBlock.Lifetime)
	serialisedFileName := fmt.Sprintf("%x", sha256.Sum256([]byte(bundle.ID().String())))
	bd := BundleDescriptor{
		ID:                   bundle.ID(),
		IDString:             bundle.ID().String(),
		Source:               bundle.PrimaryBlock.SourceNode,
		Destination:          bundle.PrimaryBlock.Destination,
		ReportTo:             bundle.PrimaryBlock.ReportTo,
		AlreadySentTo:        []bpv7.EndpointID{bst.nodeID},
		RetentionConstraints: []Constraint{DispatchPending},
		Retain:               true,
		Dispatch:             true,
		Expires:              bundle.PrimaryBlock.CreationTimestamp.DtnTime().Time().Add(lifetimeDuration),
		SerialisedFileName:   serialisedFileName,
		Bundle:               nil,
	}

	if previousNodeBlock, err := bundle.ExtensionBlockByType(bpv7.BlockTypePreviousNodeBlock); err == nil {
		previousNode := previousNodeBlock.Value.(*bpv7.PreviousNodeBlock).Endpoint()
		bd.AlreadySentTo = append(bd.AlreadySentTo, previousNode)
		log.WithFields(log.Fields{
			"bundle": bd.ID,
			"sender": previousNode,
		}).Debug("Added sender to AlreadySentTo")
	}

	err := storeSingleton.metadataStore.Insert(bd.IDString, bd)
	if err != nil {
		return nil, err
	}

	serialisedPath := filepath.Join(bst.bundleDirectory, serialisedFileName)
	f, err := os.Create(serialisedPath)
	defer f.Close()
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bd.IDString,
			"error":  err,
		}).Error("Error opening file to store serialised bundle. Deleting...")
		delErr := bst.DeleteBundle(&bd)
		if delErr != nil {
			log.WithFields(log.Fields{
				"bundle": bd.IDString,
				"error":  delErr,
			}).Error("Error deleting BundleDescriptor. Something is very wrong")
			err = multierror.Append(err, delErr)
		}
		return nil, err
	}

	w := bufio.NewWriter(f)
	err = cboring.Marshal(bundle, w)
	if err != nil {
		return nil, err
	}
	err = w.Flush()

	return &bd, err
}

func (bst *BundleStore) InsertBundle(bundle *bpv7.Bundle) (*BundleDescriptor, error) {
	bd := BundleDescriptor{}
	err := bst.metadataStore.Get(bundle.ID().String(), &bd)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundle.ID().String(),
			"error":  err,
		}).Debug("Could not get bundle from store (because it may be new)")
		return bst.insertNewBundle(bundle)
	}

	log.WithField("bundle", bundle.ID().String()).Debug("Bundle already exists, updating metadata")

	var uerr error
	if previousNodeBlock, err := bundle.ExtensionBlockByType(bpv7.BlockTypePreviousNodeBlock); err == nil {
		previousNode := previousNodeBlock.Value.(*bpv7.PreviousNodeBlock).Endpoint()
		bd.AlreadySentTo = append(bd.AlreadySentTo, previousNode)
		uerr = bst.updateBundleMetadata(&bd)
	}

	return &bd, uerr
}

func (bst *BundleStore) updateBundleMetadata(bundleDescriptor *BundleDescriptor) error {
	bndl := bundleDescriptor.Bundle
	bundleDescriptor.Bundle = nil
	err := bst.metadataStore.Update(bundleDescriptor.IDString, bundleDescriptor)
	bundleDescriptor.Bundle = bndl
	return err
}

func (bst *BundleStore) DeleteBundle(bundleDescriptor *BundleDescriptor) error {
	var multiErr *multierror.Error
	multiErr = multierror.Append(multiErr, bst.metadataStore.Delete(bundleDescriptor.IDString, bundleDescriptor))
	serialisedPath := filepath.Join(bst.bundleDirectory, bundleDescriptor.SerialisedFileName)
	multiErr = multierror.Append(multiErr, os.Remove(serialisedPath))
	return multiErr.ErrorOrNil()
}

func (bst *BundleStore) GarbageCollect() {
	log.Debug("Garbage collecting store")

	now := time.Now()

	bundles := make([]BundleDescriptor, 0)
	err := bst.metadataStore.Find(&bundles, badgerhold.Where("Expires").Lt(now).And("Retain").Eq(false))
	if err != nil {
		log.WithField("error", err).Error("Error getting bundles for gc")
		return
	}
	log.WithField("bundles", bundles).Debug("Bundles ready for deletion")

	for _, bndl := range bundles {
		err = bst.DeleteBundle(&bndl)
		if err != nil {
			log.WithField("error", err).Error("Error deleting bundle")
		}
	}
}
