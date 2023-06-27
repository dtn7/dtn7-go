package store

import (
	"os"
	"reflect"
	"testing"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"pgregory.net/rapid"
)

func initTest(t *rapid.T) {
	nodeID, err := bpv7.NewEndpointID(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(t, "nodeID"))
	if err != nil {
		t.Fatal(err)
	}

	err = InitialiseStore(nodeID, "/tmp/dtn7-test")
	if err != nil {
		t.Fatal(err)
	}
}

func cleanupTest(t *rapid.T) {
	err := os.RemoveAll("/tmp/dtn7-test")
	if err != nil {
		t.Fatal(err)
	}
}

func generateBundle(t *rapid.T) bpv7.Bundle {
	// TODO: more variable data
	bndl, err := bpv7.Builder().
		CRC(bpv7.CRC32).
		Source(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(t, "source")).
		Destination(rapid.StringMatching(bpv7.DtnEndpointRegexpFull).Draw(t, "destination")).
		CreationTimestampEpoch().
		Lifetime("10m").
		HopCountBlock(64).
		BundleAgeBlock(0).
		PayloadBlock([]byte(rapid.String().Draw(t, "payload"))).
		Build()
	if err != nil {
		t.Fatalf("Error during bundle creation %s", err)
	}
	return bndl
}

func TestBundleInsertion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		initTest(t)
		defer cleanupTest(t)

		bundle := rapid.Custom(generateBundle).Draw(t, "bundle")
		bd, err := DTNStore.insertNewBundle(bundle)
		if err != nil {
			t.Fatal(err)
		}

		bdLoad, err := DTNStore.LoadBundleDescriptor(bundle.ID())
		if err != nil {
			t.Fatalf("Failed to load bundle with ID %s, error: %s", bundle.ID(), err)
		}

		if !reflect.DeepEqual(bd, bdLoad) {
			t.Fatal("Retrieved BundleDescriptor not equal")
		}
	})
}
