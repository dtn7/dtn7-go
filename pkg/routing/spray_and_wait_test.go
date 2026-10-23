package routing

import (
	"testing"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

const (
	initialCopies  uint64 = 10
	receivedCopies uint64 = 5
)

func TestSprayBasic_NotifyNewBundle(t *testing.T) {
	router := NewSprayAndWait(initialCopies, false)
	setup(t, router)
	defer teardown(t)

	descriptor, bundle := testBundle(t)

	router.NotifyNewBundle(descriptor, bundle)

	data, ok := descriptor.GetMiscData(sprayBundleCopiesKey)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	copies := data.(uint64)

	if copies != initialCopies {
		t.Fatalf("Spray&Wait did not set correct initial copies. Wanted: %d, got: %d", initialCopies, copies)
	}
}

func TestSprayBinary_NotifyNewBundle(t *testing.T) {
	router := NewSprayAndWait(initialCopies, true)
	setup(t, router)
	defer teardown(t)

	descriptor, bundle := testBundle(t)

	router.NotifyNewBundle(descriptor, bundle)

	data, ok := descriptor.GetMiscData(sprayBundleCopiesKey)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	copies := data.(uint64)

	if copies != initialCopies {
		t.Fatalf("Spray&Wait did not set correct initial copies. Wanted: %d, got: %d", initialCopies, copies)
	}
}

func TestSprayBasic_NotifyReceivedBundle(t *testing.T) {
	router := NewSprayAndWait(initialCopies, false)
	setup(t, router)
	defer teardown(t)

	// test bundle with no BinarySprayBlock
	descriptor, bundle := testBundle(t)

	router.NotifyReceivedBundle(descriptor, bundle)

	copies, ok := getSprayCopies(descriptor)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	if copies != 0 {
		t.Fatalf("Spray&Wait did not set correct initial copies. Wanted: %d, got: %d", 0, copies)
	}

	// test bundle with BinarySprayBlock
	descriptor, bundle = testBundle(t)
	block := bpv7.NewCanonicalBlock(0, 0, bpv7.NewBinarySprayBlock(receivedCopies))
	err := bundle.AddExtensionBlock(block)
	if err != nil {
		t.Fatalf("Error adding BinarySprayBlock to bundle: %s", err)
	}

	router.NotifyReceivedBundle(descriptor, bundle)

	copies, ok = getSprayCopies(descriptor)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	if copies != receivedCopies {
		t.Fatalf("Spray&Wait did not set correct initial copies. Wanted: %d, got: %d", receivedCopies, copies)
	}
}

func TestSprayBinary_NotifyReceivedBundle(t *testing.T) {
	router := NewSprayAndWait(initialCopies, true)
	setup(t, router)
	defer teardown(t)

	// test bundle with no BinarySprayBlock
	descriptor, bundle := testBundle(t)

	router.NotifyReceivedBundle(descriptor, bundle)

	copies, ok := getSprayCopies(descriptor)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	if copies != 0 {
		t.Fatalf("Spray&Wait did not set correct initial copies. Wanted: %d, got: %d", 0, copies)
	}

	// test bundle with BinarySprayBlock
	descriptor, bundle = testBundle(t)
	block := bpv7.NewCanonicalBlock(0, 0, bpv7.NewBinarySprayBlock(receivedCopies))
	err := bundle.AddExtensionBlock(block)
	if err != nil {
		t.Fatalf("Error adding BinarySprayBlock to bundle: %s", err)
	}

	router.NotifyReceivedBundle(descriptor, bundle)

	copies, ok = getSprayCopies(descriptor)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	if copies != receivedCopies {
		t.Fatalf("Spray&Wait did not set correct initial copies. Wanted: %d, got: %d", receivedCopies, copies)
	}
}

func TestSprayBasic_SelectPeersForForwarding(t *testing.T) {
	router := NewSprayAndWait(initialCopies, false)
	setup(t, router)
	defer teardown(t)

	peers := generatePeers(15)

	descriptor, bundle := testBundle(t)
	router.NotifyNewBundle(descriptor, bundle)

	// feeding it 5 peers, should select all 5 and reduce the remaining copies by 5
	selectedPeers := router.SelectPeersForForwarding(descriptor, peers[:5])
	if len(selectedPeers) != 5 {
		t.Fatal("Spray&Wait selected wrong number of peers")
	}

	copies, ok := getSprayCopies(descriptor)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	if copies != 5 {
		t.Fatal("Bundle should have 5 copies left")
	}

	// add previously selected peers to bundle known holders
	for _, peer := range selectedPeers {
		err := descriptor.AddKnownHolder(peer.GetPeerEndpointID())
		if err != nil {
			t.Fatalf("Error adding peer to known holders: %s", err)
		}
	}

	// feeding it 8, of which 5 are the same as before, should just select the 3 new peers and reduce the remaining copies by 3.
	// The processing pipeline filters out known holders before asking the routing algorithm,
	// so we do the same here.
	selectedPeers = router.SelectPeersForForwarding(descriptor, withoutKnownHolders(t, descriptor, peers[:8]))

	if len(selectedPeers) != 3 {
		t.Fatalf("Spray&Wait selected wrong number of peers. Expected 3, got %d", len(selectedPeers))
	}

	copies, ok = getSprayCopies(descriptor)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	if copies != 2 {
		t.Fatal("Bundle should have 2 copies left")
	}

	// add previously selected peers to bundle known holders
	for _, peer := range selectedPeers {
		err := descriptor.AddKnownHolder(peer.GetPeerEndpointID())
		if err != nil {
			t.Fatalf("Error adding peer to known holders: %s", err)
		}
	}

	// feeding it all peers should now only select 2 and drop the remaining copies to 0
	selectedPeers = router.SelectPeersForForwarding(descriptor, withoutKnownHolders(t, descriptor, peers))

	if len(selectedPeers) != 2 {
		t.Fatalf("Spray&Wait selected wrong number of peers. Expected 2, got %d", len(selectedPeers))
	}

	copies, ok = getSprayCopies(descriptor)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	if copies != 0 {
		t.Fatal("Bundle should have 0 copies left")
	}

	// trying again should return no peers
	selectedPeers = router.SelectPeersForForwarding(descriptor, withoutKnownHolders(t, descriptor, peers))

	if len(selectedPeers) != 0 {
		t.Fatalf("Spray&Wait selected wrong number of peers. Expected 0, got %d", len(selectedPeers))
	}
}

func TestSprayBinary_SelectPeersForForwarding(t *testing.T) {
	router := NewSprayAndWait(initialCopies, true)
	setup(t, router)
	defer teardown(t)

	peers := generatePeers(15)

	descriptor, bundle := testBundle(t)
	router.NotifyNewBundle(descriptor, bundle)

	selectedPeers := router.SelectPeersForForwarding(descriptor, peers)

	if len(selectedPeers) != 4 {
		t.Fatalf("Binary spray selected wrong number of peers. Expected: 5, got %v", len(selectedPeers))
	}

	err := descriptor.AddKnownHolder(peers[0].GetPeerEndpointID())
	if err != nil {
		t.Fatalf("Error adding peer to known holders: %s", err)
	}

	copies, ok := getSprayCopies(descriptor)
	if !ok {
		t.Fatalf("Could not retrieve bundle copies")
	}

	if copies != 1 {
		t.Fatal("Bundle should have 1 copy left")
	}

	bundleCopies, present := router.bundleCopies[descriptor.ID()]
	if !present {
		t.Fatal("Did not save bundle copies")
	}

	if len(bundleCopies) != len(selectedPeers) {
		t.Fatal("Did not save number of copies for each peer")
	}

	var total uint64 = 0
	for _, peer := range selectedPeers {
		peerCopies, present := bundleCopies[peer.GetPeerEndpointID()]
		if !present {
			t.Fatalf("Did not save copies for peer %v", peer)
		}
		total += peerCopies
	}

	if total != 9 {
		t.Fatal("Gave out wrong number of copies")
	}

	// trying again should return no peers
	selectedPeers = router.SelectPeersForForwarding(descriptor, peers)
	if len(selectedPeers) != 0 {
		t.Fatal("Should not have selected any peers")
	}
}

func TestSprayBinary_ModifyHeaders(t *testing.T) {
	router := NewSprayAndWait(initialCopies, true)
	setup(t, router)
	defer teardown(t)

	peer := generatePeers(1)[0]
	descriptor, bundle := testBundle(t)

	router.bundleCopies[descriptor.ID()] = make(map[bpv7.EndpointID]uint64)
	router.bundleCopies[descriptor.ID()][peer.GetPeerEndpointID()] = 6
	bundleHeaders := bpv7.BundleHeaders(bundle)

	err := router.ModifyHeaders(descriptor, bundleHeaders, peer)
	if err != nil {
		t.Fatal(err)
	}

	block, err := bundleHeaders.ExtensionBlockByType(bpv7.BlockTypeBinarySprayBlock)
	if err != nil {
		t.Fatal("Headers should contain BinarySprayBlock")
	}

	copies := block.Value.(*bpv7.BinarySprayBlock).Copies()
	if copies != 6 {
		t.Error("BinarySprayBlock does not contain correct number of copies")
	}

	_, present := router.bundleCopies[descriptor.ID()][peer.GetPeerEndpointID()]
	if present {
		t.Error("Copies for peer should have been removed")
	}
}
