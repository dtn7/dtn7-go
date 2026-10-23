package routing

import (
	"fmt"
	"testing"

	"github.com/dtn7/dtn7-go/internal/testhelpers"
	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
	"github.com/dtn7/dtn7-go/pkg/cla/dummy_cla"
	"github.com/dtn7/dtn7-go/pkg/store"
)

func setup(t *testing.T, router Algorithm) {
	nodeID := bpv7.EndpointID{EndpointType: bpv7.DtnEndpoint{
		NodeName:  "test",
		Demux:     "",
		IsDtnNone: false,
	}}
	err := store.InitialiseStore(nodeID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	InitialiseAlgorithm(router)
}

func teardown(t *testing.T) {
	err := store.ShutdownStore()
	if err != nil {
		t.Fatal(err)
	}
	ShutdownAlgorithm()
}

func generatePeers(n uint) []cla.ConvergenceSender {
	peers := make([]cla.ConvergenceSender, n)
	for i := range peers {
		eid := bpv7.EndpointID{
			EndpointType: bpv7.DtnEndpoint{
				NodeName:  fmt.Sprintf("test_%d", i),
				Demux:     "",
				IsDtnNone: false,
			},
		}
		peers[i] = dummy_cla.NewSuperDummyCLA(eid)
	}
	return peers
}

// withoutKnownHolders filters out all peers which are already listed as known holders of the given bundle.
// This mirrors what the processing pipeline does before calling SelectPeersForForwarding.
func withoutKnownHolders(t *testing.T, descriptor *store.BundleDescriptor, peers []cla.ConvergenceSender) []cla.ConvergenceSender {
	t.Helper()

	knownHolders, err := descriptor.GetKnownHolders()
	if err != nil {
		t.Fatalf("Error getting known holders: %s", err)
	}

	filtered := make([]cla.ConvergenceSender, 0, len(peers))
	for _, peer := range peers {
		known := false
		for _, eid := range knownHolders {
			if peer.GetPeerEndpointID() == eid {
				known = true
				break
			}
		}

		if !known {
			filtered = append(filtered, peer)
		}
	}

	return filtered
}

func testBundle(t *testing.T) (*store.BundleDescriptor, *bpv7.Bundle) {
	bundle := testhelpers.SampleBundle(t)

	descriptor, err := store.GetStoreSingleton().InsertBundle(bundle)
	if err != nil {
		t.Fatalf("Error inserting bundle into store %s", err)
	}

	return descriptor, bundle
}
