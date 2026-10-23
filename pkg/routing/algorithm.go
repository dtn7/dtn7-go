// SPDX-FileCopyrightText: 2023, 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package routing provides and interface & implementations for routing algorithms.
//
// Since there should only be a single Algorithm active at any time, this package employs the singleton pattern.
// Use `InitialiseAlgorithm` and `GetAlgorithmSingleton.`
package routing

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
	"github.com/dtn7/dtn7-go/pkg/store"
)

type AlgorithmEnum uint32

const (
	AlgorithmEpidemic     AlgorithmEnum = 1
	AlgorithmSprayAndWait AlgorithmEnum = 2
)

func AlgorithmEnumFromString(name string) (AlgorithmEnum, error) {
	switch name = strings.ToLower(name); name {
	case "epidemic":
		return AlgorithmEpidemic, nil
	case "spray&wait":
		return AlgorithmSprayAndWait, nil
	default:
		return 0, fmt.Errorf("%s is not a valid algorithm name", name)
	}
}

// Algorithm is an interface to specify routing algorithms for delay-tolerant networks.
type Algorithm interface {
	// NotifyNewBundle notifies an Algorithm about new bundles.
	// Called when a new bundle was created on this node
	// Whether an algorithm acts on this information or ignores it, is an implementation matter.
	NotifyNewBundle(descriptor *store.BundleDescriptor, bundle *bpv7.Bundle)

	// NotifyReceivedBundle notifies an Algorithm about a bundle received from another node
	// Whether an algorithm acts on this information or ignores it, is an implementation matter.
	NotifyReceivedBundle(descriptor *store.BundleDescriptor, bundle *bpv7.Bundle)

	// NotifyReceivedAdministrativeRecord notifies the algorithm about a bundle that contains an administrative record
	// Whether an algorithm acts on this information or ignores it, is an implementation matter.
	// The return value "retain" tells the processing pipeline whether the bundle should be saved to disk, or dropped after processing
	NotifyReceivedAdministrativeRecord(bundle *bpv7.Bundle) (retain bool)

	// SelectPeersForForwarding returns an array of ConvergenceSender for a requested bundle.
	// dtnd will attempt to forward the bundle to all the selected peers.
	// The peers passed in are already filtered: they only contain nodes which are not known to
	// already hold the bundle. Implementations must not assume they need to filter known holders themselves.
	SelectPeersForForwarding(descriptor *store.BundleDescriptor, peers []cla.ConvergenceSender) []cla.ConvergenceSender

	// ModifyHeaders is called by the processing pipeline after SelectPeersForForwarding.
	// It is called once for every peer that was selected (which is passed in the "peer" argument).
	// The "headers" argument is a pointer to a copy of the bundle's primary and extension-blocks.
	// Once this function returns, a new bundle will be constructed by combining the headers with the bundle's payload
	// this is the bundle that will be sent to the peer.
	// This allows the algorithm to modify the bundle's headers before it is sent.
	//
	// IMPORTANT: depending on the underlying implementation, any CanonicalBlock might hold a pointer to an ExtensionBlock.
	// So you should NEVER modify existing blocks.
	// Instead, use RemoveExtensionBlocks and AddExtensionBlock to remove and (re)add blocks
	ModifyHeaders(descriptor *store.BundleDescriptor, headers *bpv7.PartialBundle, peer cla.ConvergenceSender) error

	// NotifyPeerAppeared notifies the Algorithm about a new peer.
	NotifyPeerAppeared(peer bpv7.EndpointID)

	// NotifyPeerDisappeared notifies the Algorithm about the
	// disappearance of a peer.
	NotifyPeerDisappeared(peer bpv7.EndpointID)
}

var algorithmSingleton Algorithm

func InitialiseAlgorithm(algorithm Algorithm) {
	if algorithmSingleton != nil {
		log.Fatalf("Attempting to initialise an already initialised algorithm. This must never happen!")
	}
	if algorithm == nil {
		log.Fatalf("Attempting to initialise algorithm with nil. This must never happen!")
	}

	algorithmSingleton = algorithm
}

// GetAlgorithmSingleton returns the routing algorithm singleton-instance.
// Attempting to call this function before algorithm initialisation will cause the program to panic.
func GetAlgorithmSingleton() Algorithm {
	if algorithmSingleton == nil {
		log.Fatalf("Attempting to access an uninitialised algorithm. This must never happen!")
	}
	return algorithmSingleton
}

func ShutdownAlgorithm() {
	algorithmSingleton = nil
}

// uniquePeers filters a list of ConvergenceSenders for uniqueness.
// Sometimes you may have multiple CLAs which connect ot the same peer, and you may or may not want to send a bundle across all parallel links.
func uniquePeers(peers []cla.ConvergenceSender) []cla.ConvergenceSender {
	endpoints := make(map[bpv7.EndpointID]bool)
	unique := make([]cla.ConvergenceSender, 0, len(peers))
	for _, sender := range peers {
		_, present := endpoints[sender.GetPeerEndpointID()]
		if !present {
			endpoints[sender.GetPeerEndpointID()] = true
			unique = append(unique, sender)
		}
	}
	return unique
}
