package store

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"github.com/dtn7/cboring"
	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/hashicorp/go-multierror"
	log "github.com/sirupsen/logrus"
	"github.com/timshannon/badgerhold/v4"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	metadataStore   *badgerhold.Store
	bundleDirectory string
}

var DTNStore *Store

func InitialiseStore(path string) error {
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

	DTNStore = &Store{metadataStore: badgerStore, bundleDirectory: bundleDirectory}

	return nil
}

func (store *Store) Close() error {
	return store.metadataStore.Close()
}

func (store *Store) LoadBundleDescriptor(bundleId bpv7.BundleID) (*BundleDescriptor, error) {
	idString := bundleId.String()
	var bd *BundleDescriptor
	err := store.metadataStore.Get(idString, bd)
	return bd, err
}

func (store *Store) loadEntireBundle(filename string) (*bpv7.Bundle, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bundle, err := bpv7.ParseBundle(f)

	return &bundle, nil
}

func (store *Store) insertNewBundle(bundle *bpv7.Bundle) error {
	log.WithField("bundle", bundle.ID().String()).Debug("Inserting new bundle")
	lifetimeDuration := time.Millisecond * time.Duration(bundle.PrimaryBlock.Lifetime)
	serialisedFileName := fmt.Sprintf("%x", sha256.Sum256([]byte(bundle.ID().String())))
	bd := BundleDescriptor{
		ID:                   bundle.ID(),
		idString:             bundle.ID().String(),
		Source:               bundle.PrimaryBlock.SourceNode,
		Destination:          bundle.PrimaryBlock.Destination,
		ReportTo:             bundle.PrimaryBlock.ReportTo,
		alreadySentTo:        make([]bpv7.EndpointID, 0),
		retentionConstraints: []Constraint{DispatchPending},
		retain:               false,
		expires:              bundle.PrimaryBlock.CreationTimestamp.DtnTime().Time().Add(lifetimeDuration),
		serialisedFileName:   serialisedFileName,
		bundle:               nil,
	}

	if previousNodeBlock, err := bundle.ExtensionBlock(bpv7.ExtBlockTypePreviousNodeBlock); err == nil {
		previousNode := previousNodeBlock.Value.(*bpv7.PreviousNodeBlock).Endpoint()
		bd.alreadySentTo = append(bd.alreadySentTo, previousNode)
	}

	err := DTNStore.metadataStore.Insert(bd.idString, bd)
	if err != nil {
		return err
	}

	serialisedPath := filepath.Join(store.bundleDirectory, serialisedFileName)
	f, err := os.Create(serialisedPath)
	defer f.Close()
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bd.idString,
			"error":  err,
		}).Error("Error opening file to store serialised bundle. Deleting...")
		delErr := store.DeleteBundle(&bd)
		if delErr != nil {
			log.WithFields(log.Fields{
				"bundle": bd.idString,
				"error":  delErr,
			}).Error("Error deleting BundleDescriptor. Something is very wrong")
			err = multierror.Append(err, delErr)
		}
		return err
	}

	w := bufio.NewWriter(f)
	err = cboring.Marshal(bundle, w)

	return err
}

func (store *Store) InsertBundle(bundle *bpv7.Bundle) error {
	var bd *BundleDescriptor
	err := DTNStore.metadataStore.Get(bundle.ID().String(), bd)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundle.ID().String(),
			"error":  err,
		}).Debug("Could not get bundle from store (because it may be new)")
		return DTNStore.insertNewBundle(bundle)
	}

	log.WithField("bundle", bundle.ID().String()).Debug("Bundle already exists, updating metadata")

	if previousNodeBlock, err := bundle.ExtensionBlock(bpv7.ExtBlockTypePreviousNodeBlock); err == nil {
		previousNode := previousNodeBlock.Value.(*bpv7.PreviousNodeBlock).Endpoint()
		bd.alreadySentTo = append(bd.alreadySentTo, previousNode)
		return store.updateBundleMetadata(bd)
	}

	return nil
}

func (store *Store) updateBundleMetadata(bundleDescriptor *BundleDescriptor) error {
	bundlePointer := bundleDescriptor.bundle
	bundleDescriptor.bundle = nil
	err := store.metadataStore.Update(bundleDescriptor.idString, bundleDescriptor)
	bundleDescriptor.bundle = bundlePointer
	return err
}

func (store *Store) DeleteBundle(bundleDescriptor *BundleDescriptor) error {
	err := store.metadataStore.Delete(bundleDescriptor.idString, bundleDescriptor)
	err = multierror.Append(os.Remove(bundleDescriptor.serialisedFileName))
	return err
}
