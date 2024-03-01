package processing

import (
	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
	"github.com/dtn7/dtn7-ng/pkg/routing"
	"github.com/dtn7/dtn7-ng/pkg/store"
	log "github.com/sirupsen/logrus"
	"sync"
)

var ownNodeID bpv7.EndpointID

func SetOwnNodeID(nid bpv7.EndpointID) {
	ownNodeID = nid
}

// forwardingAsync implements the bundle forwarding procedure described in RFC9171 section 5.4
func forwardingAsync(bundleDescriptor *store.BundleDescriptor) {
	log.WithField("bundle", bundleDescriptor.ID.String()).Debug("Processing bundle")

	// Step 1: add "Forward Pending, remove "Dispatch Pending"
	err := bundleDescriptor.AddConstraint(store.ForwardPending)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Error adding constraint to bundle")
		return
	}
	err = bundleDescriptor.RemoveConstraint(store.DispatchPending)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Error removing constraint from bundle")
		return
	}

	// Step 2: determine if contraindicated - whatever that means
	// Step 2.1: Call routing algorithm(?)
	forwardToPeers := routing.GetAlgorithmSingleton().SelectPeersForForwarding(bundleDescriptor)

	// Step 3: if contraindicated, call `contraindicateBundle`, and return
	if len(forwardToPeers) == 0 {
		bundleContraindicated(bundleDescriptor)
		return
	}

	// Step 4:
	bundle, err := bundleDescriptor.Load()
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Error loading bundle from disk")
		return
	}
	// Step 4.1: remove previous node block
	if prevNodeBlock, err := bundle.ExtensionBlock(bpv7.ExtBlockTypePreviousNodeBlock); err == nil {
		bundle.RemoveExtensionBlockByBlockNumber(prevNodeBlock.BlockNumber)
	}
	// Step 4.2: add new previous node block
	prevNodeBlock := bpv7.NewCanonicalBlock(0, 0, bpv7.NewPreviousNodeBlock(ownNodeID))
	err = bundle.AddExtensionBlock(prevNodeBlock)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Error adding PreviousNodeBlock to bundle")
	}
	// TODO: Step 4.3: update bundle age block
	// Step 4.4: call CLAs for transmission
	var mutex sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(forwardToPeers))
	for _, peer := range forwardToPeers {
		go forwardBundleToPeer(&mutex, bundleDescriptor, bundle, peer, &wg)
	}
	wg.Wait()

	// Step 6: remove "Forward Pending"
	err = bundleDescriptor.RemoveConstraint(store.ForwardPending)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Error removing constraint from bundle")
		return
	}
}

func BundleForwarding(bundleDescriptor *store.BundleDescriptor) {
	forwardingAsync(bundleDescriptor)
}

func bundleContraindicated(bundleDescriptor *store.BundleDescriptor) {
	// TODO: is there anything else to do here?
	err := bundleDescriptor.ResetConstraints()
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Error resetting bundle constraints")
	}
}

func forwardBundleToPeer(mutex *sync.Mutex, bundleDescriptor *store.BundleDescriptor, bundle bpv7.Bundle, peer cla.ConvergenceSender, wg *sync.WaitGroup) {
	log.WithFields(log.Fields{
		"bundle": bundle.ID(),
		"cla":    peer,
	}).Info("Sending bundle to a CLA (ConvergenceSender)")

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
		mutex.Lock()
		bundleDescriptor.AddAlreadySent(peer.GetPeerEndpointID())
		mutex.Unlock()
	}
	wg.Done()
}

func DispatchPending() {
	log.Debug("Dispatching bundles")

	bndls, err := store.GetStoreSingleton().GetDispatchable()
	if err != nil {
		log.WithError(err).Error("Error dispatching pending bundles")
		return
	}
	log.WithField("bundles", bndls).Debug("Bundles to dispatch")

	for _, bndl := range bndls {
		BundleForwarding(bndl)
	}
}

func NewPeer(peerID bpv7.EndpointID) {
	routing.GetAlgorithmSingleton().NotifyPeerAppeared(peerID)
	DispatchPending()
}
