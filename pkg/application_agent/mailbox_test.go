package application_agent

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"pgregory.net/rapid"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/store"
)

const mailboxTestFolderFormat = "/tmp/dtn7-go-tests/mailbox-tests/%v"

func initMailboxTest(t *testing.T) (string, *Mailbox) {
	instanceID := fmt.Sprintf("%v%v", rand.Uint64(), rand.Uint64())
	nodeID, _ := bpv7.NewEndpointID("dtn://test/")

	err := store.InitialiseStore(nodeID, fmt.Sprintf(mailboxTestFolderFormat, instanceID))
	if err != nil {
		t.Fatal(err)
	}

	mailbox := NewMailbox()

	return instanceID, mailbox
}

func cleanupMailboxTest(t *testing.T, instanceID string) {
	err := store.GetStoreSingleton().Shutdown()
	if err != nil {
		t.Fatal(err)
	}
	err = os.RemoveAll(fmt.Sprintf(mailboxTestFolderFormat, instanceID))
	if err != nil {
		t.Fatal(err)
	}
}

func TestMailbox_Deliver(t *testing.T) {
	rapid.Check(t, func(tr *rapid.T) {
		instanceID, mailbox := initMailboxTest(t)
		defer cleanupMailboxTest(t, instanceID)

		bndl := bpv7.GenerateRandomizedBundle(tr, 0)

		bdesc, err := store.GetStoreSingleton().InsertBundle(bndl)
		if err != nil {
			tr.Fatal(err)
		}

		err = mailbox.Deliver(bdesc)
		if err != nil {
			tr.Fatal(errors.Unwrap(err))
		}

		retrieved, ok := mailbox.messages[bndl.ID()]
		if !ok {
			tr.Fatal("Delivered bundle not in messages-dict")
		}
		if retrieved {
			tr.Fatal("Unretrieved bundle marked as retrieved")
		}
	})
}

func TestMailbox_Get(t *testing.T) {
	rapid.Check(t, func(tr *rapid.T) {
		instanceID, mailbox := initMailboxTest(t)
		defer cleanupMailboxTest(t, instanceID)

		nBundles := rapid.Uint8Min(1).Draw(tr, "Drawing number of test bundles")
		for i := range nBundles {
			bndl := bpv7.GenerateRandomizedBundle(tr, i)

			bdesc, err := store.GetStoreSingleton().InsertBundle(bndl)
			if err != nil {
				tr.Fatal(err)
			}
			err = mailbox.Deliver(bdesc)
			if err != nil {
				tr.Fatal(errors.Unwrap(err))
			}

			retrieved, err := mailbox.Get(bndl.ID(), false)
			if err != nil {
				tr.Fatal(errors.Unwrap(err))
			}

			if !reflect.DeepEqual(*bndl, *retrieved) {
				tr.Fatal("Retrieved bundle was not the same")
			}

			if _, ok := mailbox.messages[bndl.ID()]; !ok {
				tr.Fatal("Bundle erroneously removed")
			}

			if !mailbox.messages[bndl.ID()] {
				tr.Fatal("Retrieved bundle not marked as retrieved")
			}

			_, err = mailbox.Get(bndl.ID(), true)

			if _, ok := mailbox.messages[bndl.ID()]; ok {
				tr.Fatal("Bundle should have been removed")
			}
		}
	})
}

func TestMailbox_All(t *testing.T) {
	rapid.Check(t, func(tr *rapid.T) {
		instanceID, mailbox := initMailboxTest(t)
		defer cleanupMailboxTest(t, instanceID)

		nBundles := rapid.Uint8Min(1).Draw(tr, "Drawing number of test bundles")
		bundles := make([]*bpv7.Bundle, 0, nBundles)
		descriptors := make(map[bpv7.BundleID]*store.BundleDescriptor, nBundles)
		bundleMap := make(map[bpv7.BundleID]*bpv7.Bundle, nBundles)

		for i := range nBundles {
			bndl := bpv7.GenerateRandomizedBundle(tr, i)
			if _, ok := bundleMap[bndl.ID()]; !ok {
				bundles = append(bundles, bndl)
				bundleMap[bndl.ID()] = bndl
				bdesc, err := store.GetStoreSingleton().InsertBundle(bndl)
				if err != nil {
					tr.Fatal(err)
				}
				descriptors[bndl.ID()] = bdesc
			}
		}

		setA := bundles[0 : nBundles/2]
		setB := bundles[nBundles/2:]

		for _, bndl := range setA {
			err := mailbox.Deliver(descriptors[bndl.ID()])
			if err != nil {
				tr.Fatal(err)
			}
		}

		list := mailbox.List()
		if len(setA) != len(list) {
			tr.Fatalf("List returned wrond number of ids, expected: %v, got: %v", len(setA), len(list))
		}

		get, err := mailbox.GetAll(true)
		if err != nil {
			tr.Fatal(err)
		}
		testHelperCompareLists(tr, setA, get)

		list = mailbox.List()
		if len(list) > 0 {
			tr.Fatalf("List should retrun empty slice, returned %v", list)
		}

		for _, bndl := range setB {
			err := mailbox.Deliver(descriptors[bndl.ID()])
			if err != nil {
				tr.Fatal(err)
			}
		}

		list = mailbox.List()
		if len(setB) != len(list) {
			tr.Fatalf("List returned wrond number of ids, expected: %v, got: %v", len(setB), len(list))
		}

		get, err = mailbox.GetAll(false)
		if err != nil {
			tr.Fatal(err)
		}
		testHelperCompareLists(tr, setB, get)

		list = mailbox.List()
		if len(setB) != len(list) {
			tr.Fatalf("List returned wrond number of ids, expected: %v, got: %v", len(setB), len(list))
		}
	})
}

func testHelperCompareLists(tr *rapid.T, listA, listB []*bpv7.Bundle) {
	if len(listA) != len(listB) {
		tr.Fatalf("List length mismatch: listA: %v, listB: %v", len(listA), len(listB))
	}

	for _, bndlA := range listA {
		found := false
		for _, bndlB := range listB {
			if reflect.DeepEqual(bndlA, bndlB) {
				found = true
				break
			}
		}
		if !found {
			tr.Fatalf("Bundle not present in list B: %v", bndlA)
		}
	}
}
