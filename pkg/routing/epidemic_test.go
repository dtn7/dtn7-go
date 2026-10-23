package routing

import (
	"testing"
)

func TestEpidemicRouting_SelectPeersForForwarding(t *testing.T) {
	router := NewEpidemicRouting()
	setup(t, router)
	defer teardown(t)

	peers := generatePeers(10)

	descriptor, _ := testBundle(t)

	// send to half of peers
	selectedPeers := router.SelectPeersForForwarding(descriptor, peers[:5])

	if len(selectedPeers) != 5 {
		t.Fatal("Epidemic did not select correct number of peers")
	}

	for i := range selectedPeers {
		if selectedPeers[i] != peers[i] {
			t.Fatal("Epidemic did not select correct peer")
		}
	}

	// add previously selected peers to bundle known holders
	for _, peer := range selectedPeers {
		err := descriptor.AddKnownHolder(peer.GetPeerEndpointID())
		if err != nil {
			t.Fatalf("Error adding peer to known holders: %s", err)
		}
	}

	// giving the same peers again should select none.
	// The processing pipeline filters out known holders before asking the routing algorithm,
	// so we do the same here.
	selectedPeers = router.SelectPeersForForwarding(descriptor, withoutKnownHolders(t, descriptor, peers[:5]))

	if len(selectedPeers) != 0 {
		t.Fatal("Epidemic should not select any peers")
	}

	// giving all peers should only select new peers
	selectedPeers = router.SelectPeersForForwarding(descriptor, withoutKnownHolders(t, descriptor, peers))

	if len(selectedPeers) != 5 {
		t.Fatal("Epidemic did not select correct number of peers")
	}

	for i := range selectedPeers {
		if selectedPeers[i] != peers[i+5] {
			t.Fatal("Epidemic did not select correct peer")
		}
	}
}
