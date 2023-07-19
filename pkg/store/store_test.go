package store

import (
	"fmt"
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
	err := GetStoreSingleton().Close()
	if err != nil {
		t.Fatal(err)
	}
	err = os.RemoveAll("/tmp/dtn7-test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestBundleInsertion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		initTest(t)
		defer cleanupTest(t)

		bundle := bpv7.GenerateBundle(t, 0)
		bd, err := GetStoreSingleton().insertNewBundle(bundle)
		if err != nil {
			t.Fatal(err)
		}

		bdLoad, err := GetStoreSingleton().LoadBundleDescriptor(bundle.ID())
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(bd, bdLoad) {
			t.Fatal("Retrieved BundleDescriptor not equal")
		}

		bundleLoad, err := bdLoad.Load()
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(bundle, bundleLoad) {
			t.Fatal("Retrieved Bundle not equal")
		}
	})
}

func TestConstraints(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		initTest(t)
		defer cleanupTest(t)

		bundle := bpv7.GenerateBundle(t, 0)
		bd, err := GetStoreSingleton().insertNewBundle(bundle)
		if err != nil {
			t.Fatal(err)
		}

		numConstraints := rapid.IntRange(1, 5).Draw(t, "Number of constraints")
		constraints := make([]Constraint, numConstraints)
		for i := range constraints {
			constraint := Constraint(rapid.IntRange(int(DispatchPending), int(ReassemblyPending)).Draw(t, fmt.Sprintf("constraint %v", i)))
			constraints[i] = constraint
		}

		// test constraint addition
		addConstraints(t, bd, constraints)
		// test constraint deletion
		removeConstraints(t, bd, constraints)

		// test constraint reset
		addConstraints(t, bd, constraints)
		err = bd.ResetConstraints()
		if err != nil {
			t.Fatal(err)
		}
		bdLoad, err := GetStoreSingleton().LoadBundleDescriptor(bd.ID)
		if err != nil {
			t.Fatal(err)
		}
		if bdLoad.Retain || len(bdLoad.RetentionConstraints) > 0 {
			t.Fatal("RetentionConstraint reset failed")
		}
	})
}

func addConstraints(t *rapid.T, bd *BundleDescriptor, constraints []Constraint) {
	for _, constraint := range constraints {
		err := bd.AddConstraint(constraint)
		if err != nil {
			t.Fatal(err)
		}
		bdLoad, err := GetStoreSingleton().LoadBundleDescriptor(bd.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !(len(bdLoad.RetentionConstraints) > 0) {
			t.Fatal("Retention constraints empty after addition")
		}
		if !bdLoad.Retain {
			t.Fatal("Retention-flag not set after addition")
		}
		if !(bdLoad.RetentionConstraints[len(bdLoad.RetentionConstraints)-1] == constraint) {
			t.Fatalf("Constraint %v not in descriptor constraints %v", constraint, bdLoad.RetentionConstraints)
		}
	}
}

func removeConstraints(t *rapid.T, bd *BundleDescriptor, constraints []Constraint) {
	for _, constraint := range constraints {
		err := bd.RemoveConstraint(constraint)
		if err != nil {
			t.Fatal(err)
		}
		bdLoad, err := GetStoreSingleton().LoadBundleDescriptor(bd.ID)
		if err != nil {
			t.Fatal(err)
		}

		if (len(bdLoad.RetentionConstraints) == 0) && bdLoad.Retain {
			t.Fatal("Retention flag still set after all constraints removed")
		}

		for _, conLoad := range bdLoad.RetentionConstraints {
			if conLoad == constraint {
				t.Fatalf("Constraint %v still present after deletion: %v", constraint, bd.RetentionConstraints)
			}
		}
	}
}
