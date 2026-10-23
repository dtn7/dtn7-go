package store

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	log "github.com/sirupsen/logrus"
	"pgregory.net/rapid"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

func initTest(t *testing.T) string {
	log.SetLevel(log.ErrorLevel)
	dir := t.TempDir()
	initialiseStore(t, dir)
	return dir
}

func initialiseStore(t *testing.T, dir string) {
	nodeID := bpv7.EndpointID{EndpointType: bpv7.DtnEndpoint{
		NodeName:  "test",
		Demux:     "",
		IsDtnNone: false,
	}}

	err := InitialiseStore(nodeID, dir)
	if err != nil {
		t.Fatal(err)
	}
}

func cleanupTest(t *testing.T) {
	err := ShutdownStore()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBundleInsertion(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		initTest(t)
		defer cleanupTest(t)

		bundle := randomizedBundleGenerator.Draw(rt, "bundle")
		bd, err := GetStoreSingleton().InsertBundle(bundle)
		if err != nil {
			rt.Fatal(err)
		}

		bdLoad, err := GetStoreSingleton().GetBundleDescriptor(bundle.ID())
		if err != nil {
			rt.Fatal(err)
		}

		if !reflect.DeepEqual(bd, bdLoad) {
			rt.Fatal("Retrieved BundleDescriptor not equal")
		}

		bundleLoad, err := bdLoad.Load()
		if err != nil {
			rt.Fatal(err)
		}

		if !reflect.DeepEqual(bundle, bundleLoad) {
			rt.Fatal("Retrieved Bundle not equal")
		}
	})
}

func TestBundleDeletion(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		initTest(t)
		defer cleanupTest(t)

		bundle := randomizedBundleGenerator.Draw(rt, "bundle")
		bd, err := GetStoreSingleton().InsertBundle(bundle)
		if err != nil {
			rt.Fatal(err)
		}

		err = bd.ResetConstraints()
		if err != nil {
			rt.Fatal(err)
		}

		err = bd.Delete(false)
		if err != nil {
			rt.Fatal(err)
		}

		if !bd.Deleted() {
			rt.Fatal("Bundle not marked as deleted")
		}

		_, err = GetStoreSingleton().GetBundleDescriptor(bundle.ID())
		target := &NoSuchBundleError{}
		if !errors.As(err, &target) {
			rt.Fatal("Bundle exists after deletion")
		}
	})
}

func TestLoadFromDisk(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := initTest(t)
		defer cleanupTest(t)

		// generate bundle and add it to the store
		bundle := randomizedBundleGenerator.Draw(rt, "bundle")
		descriptor, err := GetStoreSingleton().InsertBundle(bundle)
		if err != nil {
			rt.Fatal(err)
		}

		// shut down store and restart it, so that data has to be loaded from disk
		err = ShutdownStore()
		if err != nil {
			rt.Fatal(err)
		}
		initialiseStore(t, dir)

		// get loaded descriptor & bundle and check them for equality with the original ones
		retrieved, err := GetStoreSingleton().GetBundleDescriptor(descriptor.ID())
		if err != nil {
			rt.Fatal(err)
		}

		if !(reflect.DeepEqual(descriptor.metadata, retrieved.metadata)) {
			rt.Error("Original and reloaded BundleDescriptors have different metadata")
		}

		loadBundle, err := retrieved.Load()
		if err != nil {
			rt.Fatal(err)
		}

		if !(reflect.DeepEqual(bundle, loadBundle)) {
			rt.Error("Original and reloaded BundleDescriptors have different metadata")
		}
	})
}

func TestConstraints(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		initTest(t)
		defer cleanupTest(t)

		bundle := randomizedBundleGenerator.Draw(rt, "bundle")
		bd, err := GetStoreSingleton().InsertBundle(bundle)
		if err != nil {
			rt.Fatal(err)
		}

		numConstraints := rapid.IntRange(1, 5).Draw(rt, "Number of constraints")
		constraints := make([]Constraint, numConstraints)
		for i := range constraints {
			constraint := Constraint(rapid.IntRange(int(DispatchPending), int(ReassemblyPending)).Draw(rt, fmt.Sprintf("constraint %v", i)))
			constraints[i] = constraint
		}

		// test constraint addition
		addConstraints(rt, bd, constraints)
		// test constraint deletion
		removeConstraints(rt, bd, constraints)

		// test constraint reset
		addConstraints(rt, bd, constraints)
		err = bd.ResetConstraints()
		if err != nil {
			t.Fatalf("Error resetting constraints: %v", err)
		}
		if bd.Retain() || len(bd.retentionConstraints) > 0 {
			rt.Fatal("RetentionConstraint reset failed")
		}
	})
}

func Test_loadExtensionBlocks(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		initTest(t)
		defer cleanupTest(t)
		bundle := randomizedBundleGenerator.Draw(rt, "bundle")
		bp, err := GetStoreSingleton().InsertBundle(bundle)
		if err != nil {
			return
		}

		blocks, err := bp.LoadPartialBundle(bpv7.BlockTypeHopCountBlock, bpv7.BlockTypeBundleAgeBlock)
		if !reflect.DeepEqual(bundle.PrimaryBlock, blocks.PrimaryBlock) {
			rt.Fail()
		}
		if len(blocks.ExtensionBlocks) != 2 {
			rt.Fail()
		}
		cb, err := bundle.ExtensionBlockByType(bpv7.BlockTypeHopCountBlock)
		if err != nil {
			rt.Fail()
		}
		if !slices.ContainsFunc(blocks.ExtensionBlocks, func(block bpv7.CanonicalBlock) bool {
			return reflect.DeepEqual(cb, &block)
		}) {
			rt.Fail()
		}
		cb, err = bundle.ExtensionBlockByType(bpv7.BlockTypeBundleAgeBlock)
		if err != nil {
			rt.Fail()
		}
		if !slices.ContainsFunc(blocks.ExtensionBlocks, func(block bpv7.CanonicalBlock) bool {
			return reflect.DeepEqual(cb, &block)
		}) {
			rt.Fail()
		}

		blocks, err = bp.LoadPartialBundle(bpv7.BlockTypeHopCountBlock)
		if !reflect.DeepEqual(bundle.PrimaryBlock, blocks.PrimaryBlock) {
			rt.Fail()
		}
		if len(blocks.ExtensionBlocks) != 1 {
			rt.Fail()
		}
		cb, err = bundle.ExtensionBlockByType(bpv7.BlockTypeHopCountBlock)
		if err != nil {
			rt.Fail()
		}
		if !slices.ContainsFunc(blocks.ExtensionBlocks, func(block bpv7.CanonicalBlock) bool {
			return reflect.DeepEqual(cb, &block)
		}) {
			rt.Fail()
		}

		blocks, err = bp.LoadPartialBundle(bpv7.BlockTypeBundleAgeBlock)
		if !reflect.DeepEqual(bundle.PrimaryBlock, blocks.PrimaryBlock) {
			rt.Fail()
		}
		if len(blocks.ExtensionBlocks) != 1 {
			rt.Fail()
		}
		cb, err = bundle.ExtensionBlockByType(bpv7.BlockTypeBundleAgeBlock)
		if err != nil {
			rt.Fail()
		}
		if !slices.ContainsFunc(blocks.ExtensionBlocks, func(block bpv7.CanonicalBlock) bool {
			return reflect.DeepEqual(cb, &block)
		}) {
			rt.Fail()
		}

		blocks, err = bp.LoadPartialBundle(bpv7.BlockTypePreviousNodeBlock)
		if !reflect.DeepEqual(bundle.PrimaryBlock, blocks.PrimaryBlock) {
			rt.Fail()
		}
		if len(blocks.ExtensionBlocks) != 0 {
			rt.Fail()
		}

	})
}

func addConstraints(t *rapid.T, bd *BundleDescriptor, constraints []Constraint) {
	for _, constraint := range constraints {
		err := bd.AddConstraint(constraint)
		if err != nil {
			t.Fatal(err)
		}
		if !(len(bd.retentionConstraints) > 0) {
			t.Fatal("Retention constraints empty after addition")
		}
		if !bd.Retain() {
			t.Fatal("Retention-flag not set after addition")
		}
		if !(bd.retentionConstraints[len(bd.retentionConstraints)-1] == constraint) {
			t.Fatalf("Constraint %v not in descriptor constraints %v", constraint, bd.retentionConstraints)
		}
	}
}

func removeConstraints(t *rapid.T, bd *BundleDescriptor, constraints []Constraint) {
	for _, constraint := range constraints {
		err := bd.RemoveConstraint(constraint)
		if err != nil {
			t.Fatalf("Error removing constraint: %v", err)
		}

		if (len(bd.retentionConstraints) == 0) && bd.Retain() {
			t.Fatal("Retention flag still set after all constraints removed")
		}

		for _, conLoad := range bd.retentionConstraints {
			if conLoad == constraint {
				t.Fatalf("Constraint %v still present after deletion: %v", constraint, bd.retentionConstraints)
			}
		}
	}
}
