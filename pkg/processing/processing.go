// SPDX-FileCopyrightText: 2023, 2024, 2025 Markus Sommer
// SPDX-FileCopyrightText: 2023, 2024 Artur Sterz
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package processing implements the "Bundle Processing" steps from bpv7
// See: https://www.rfc-editor.org/rfc/rfc9171.html#name-bundle-processing
package processing

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
	"github.com/dtn7/dtn7-go/pkg/routing"
	"github.com/dtn7/dtn7-go/pkg/store"
)

var ownNodeID bpv7.EndpointID

func SetOwnNodeID(nid bpv7.EndpointID) {
	ownNodeID = nid
}

// forwardingAsync implements the bundle forwarding procedure described in RFC9171 section 5.4
func forwardingAsync(bundleDescriptor *store.BundleDescriptor, bundle *bpv7.Bundle) {
	log.WithField("bundle", bundleDescriptor).Debug("Processing bundle")

	// Whatever happens below, the bundle's constraints must be reset once this pass is over.
	// Otherwise the bundle would remain in a "forwarding pending" state and would never be dispatched again.
	// Resetting instead of removing the ForwardPending constraint alone also cleans up partial state,
	// e.g. if an error occurred halfway through.
	// Note: reassembly is not implemented yet, so no ReassemblyPending constraint can be lost here.
	defer func() {
		if err := bundleDescriptor.ResetConstraints(); err != nil {
			log.WithFields(log.Fields{
				"bundle": bundleDescriptor.ID(),
				"error":  err,
			}).Debug("Error resetting bundle constraints")
		}
	}()

	// Step 1: add "Forward Pending", remove "Dispatch Pending"
	err := bundleDescriptor.AddConstraint(store.ForwardPending)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor,
			"error":  err,
		}).Error("Error adding constraint to bundle")
		return
	}
	err = bundleDescriptor.RemoveConstraint(store.DispatchPending)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor,
			"error":  err,
		}).Error("Error removing constraint from bundle")
		return
	}

	// Step 2: determine if contraindicated - whatever that means
	// Step 2.1: Call routing algorithm(?)
	knownHolders, err := bundleDescriptor.GetKnownHolders()
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID(),
			"error":  err,
		}).Warn("Could not get bundle's known holders")
		return
	}

	peers := cla.GetManagerSingleton().GetFilteredPeers(knownHolders)

	// attempt direct delivery
	peers = directDelivery(bundleDescriptor, bundle, peers)

	// Ask routing algorithm if we should send to anyone else.
	forwardToPeers := routing.GetAlgorithmSingleton().SelectPeersForForwarding(bundleDescriptor, peers)

	// Step 3: if contraindicated, the deferred ResetConstraints will make the bundle dispatchable again
	if len(forwardToPeers) == 0 {
		return
	}

	// Steps 4-6 happen in separate function
	forwardBundleToPeers(bundleDescriptor, bundle, forwardToPeers)
}

func BundleForwarding(bundleDescriptor *store.BundleDescriptor, bundle *bpv7.Bundle) {
	go forwardingAsync(bundleDescriptor, bundle)
}

// directDelivery forwards the bundle to all peers which are the bundle's destination.
// It returns the peers which were not direct-delivered to, so that the processing pipeline
// can forward the bundle to the remaining peers afterwards.
// Note: peers which failed to receive the bundle are not part of the returned list either.
// They are not added to the known holders, so the next dispatch will attempt direct delivery again.
func directDelivery(bundleDescriptor *store.BundleDescriptor, bundle *bpv7.Bundle, peers []cla.ConvergenceSender) []cla.ConvergenceSender {
	metadata, err := bundleDescriptor.Metadata()
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID(),
			"error":  err,
		}).Debug("Could not get bundle metadata")
		return peers
	}

	deliveries := make([]cla.ConvergenceSender, 0, len(peers))
	remaining := make([]cla.ConvergenceSender, 0, len(peers))

	for _, peer := range peers {
		if peer.GetPeerEndpointID() == metadata.Destination {
			deliveries = append(deliveries, peer)
		} else {
			remaining = append(remaining, peer)
		}
	}

	if len(deliveries) > 0 {
		forwardBundleToPeers(bundleDescriptor, bundle, deliveries)
	}

	return remaining
}

func forwardBundleToPeers(bundleDescriptor *store.BundleDescriptor, bundle *bpv7.Bundle, peers []cla.ConvergenceSender) {
	var err error

	// load bundle if necessary
	if bundle == nil {
		bundle, err = bundleDescriptor.Load()
		if err != nil {
			log.WithFields(log.Fields{
				"bundle": bundleDescriptor,
				"error":  err,
			}).Error("Error loading bundle from disk")
			return
		}
	}

	// Step 4:
	// Step 4.1: remove previous node block
	if prevNodeBlock, err := bundle.ExtensionBlockByType(bpv7.BlockTypePreviousNodeBlock); err == nil {
		bundle.RemoveExtensionBlockByBlockNumber(prevNodeBlock.BlockNumber)
	}
	// Step 4.2: add new previous node block
	prevNodeBlock := bpv7.NewCanonicalBlock(0, 0, bpv7.NewPreviousNodeBlock(ownNodeID))
	err = bundle.AddExtensionBlock(prevNodeBlock)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor,
			"error":  err,
		}).Error("Error adding PreviousNodeBlock to bundle")
	}
	// TODO: Step 4.3: update bundle age block
	// Step 4.4: call CLAs for transmission
	var wg sync.WaitGroup
	wg.Add(len(peers))
	for _, peer := range peers {
		go forwardBundleToPeer(bundleDescriptor, bundle, peer, &wg)
	}
	wg.Wait()
}

func forwardBundleToPeer(bundleDescriptor *store.BundleDescriptor, bundle *bpv7.Bundle, peer cla.ConvergenceSender, wg *sync.WaitGroup) {
	log.WithFields(log.Fields{
		"bundle": bundle.ID(),
		"cla":    peer,
	}).Info("Sending bundle to a CLA (ConvergenceSender)")
	defer wg.Done()

	// allow routing algorithm to modify bundle
	// "extract" headers from bundle
	bundleHeaders := bpv7.BundleHeaders(bundle)
	err := routing.GetAlgorithmSingleton().ModifyHeaders(bundleDescriptor, bundleHeaders, peer)
	if err != nil {
		// if there was an error, we abort
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID(),
			"peer":   peer.GetPeerEndpointID(),
		})
		return
	}
	// "reconstruct" bundle from modified headers
	bundle = bpv7.BundleFromPartialBundle(bundleHeaders, bundle.PayloadBlock)

	if err := peer.Send(bundle); err != nil {
		log.WithFields(log.Fields{
			"bundle": bundle.ID(),
			"cla":    peer,
			"error":  err,
		}).Warn("Sending bundle failed")
	} else {
		log.WithFields(log.Fields{
			"bundle": bundle.ID(),
			"cla":    peer,
		}).Debug("Sending bundle succeeded")
		err = bundleDescriptor.AddKnownHolder(peer.GetPeerEndpointID())
		if err != nil {
			log.WithFields(log.Fields{
				"bundle": bundle.ID(),
				"error":  err,
			}).Debug("Error adding peer to known holders")
		}
	}
}

func DispatchPending() {
	log.Debug("Dispatching bundles")

	bundles := store.GetStoreSingleton().GetDispatchable()

	log.WithField("bundles", bundles).Debug("Bundles to dispatch")

	for _, bundle := range bundles {
		BundleForwarding(bundle, nil)
	}
}

func NewPeer(peerID bpv7.EndpointID) {
	routing.GetAlgorithmSingleton().NotifyPeerAppeared(peerID)
	DispatchPending()
}
