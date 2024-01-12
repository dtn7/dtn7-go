package processing

import (
	"sync"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
	"github.com/dtn7/dtn7-ng/pkg/routing"
	"github.com/dtn7/dtn7-ng/pkg/store"
	"github.com/hashicorp/go-multierror"
	log "github.com/sirupsen/logrus"
)

var NodeID bpv7.EndpointID

// BundleForwarding implements the bundle forwarding procedure described in RFC9171 section 5.4
func BundleForwarding(bundleDescriptor *store.BundleDescriptor) error {
	// Step 1: add "Forward Pending, remove "Dispatch Pending"
	err := bundleDescriptor.AddConstraint(store.ForwardPending)
	if err != nil {
		return err
	}
	err = bundleDescriptor.RemoveConstraint(store.DispatchPending)
	if err != nil {
		return err
	}

	// Step 2: determine if contraindicated - whatever that means
	// Step 2.1: Call routing algorithm(?)
	forwardToPeers := routing.GetAlgorithmSingleton().SelectPeersForForwarding(bundleDescriptor)

	// Step 3: if contraindicated, call `contraindicateBundle`, and return
	if len(forwardToPeers) == 0 {
		return bundleContraindicated(bundleDescriptor)
	}

	// Step 4:
	bundle, err := bundleDescriptor.Load()
	if err != nil {
		return multierror.Append(err, bundleContraindicated(bundleDescriptor))
	}
	// Step 4.1: remove previous node block
	if prevNodeBlock, err := bundle.ExtensionBlock(bpv7.ExtBlockTypePreviousNodeBlock); err == nil {
		bundle.RemoveExtensionBlockByBlockNumber(prevNodeBlock.BlockNumber)
	}
	// Step 4.2: add new previous node block
	prevNodeBlock := bpv7.NewCanonicalBlock(0, 0, bpv7.NewPreviousNodeBlock(NodeID))
	err = bundle.AddExtensionBlock(prevNodeBlock)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Error adding PreviousNodeBlock to bundle")
	}
	// TODO: Step 4.3: update bundle age block
	// Step 4.4: call CLAs for transmission
	forwardBundle(bundleDescriptor, forwardToPeers)

	// Step 6: remove "Forward Pending"
	return bundleDescriptor.RemoveConstraint(store.ForwardPending)
}

func bundleContraindicated(bundleDescriptor *store.BundleDescriptor) error {
	// TODO: is there anything else to do here?
	return bundleDescriptor.ResetConstraints()
}

func forwardBundle(bundleDescriptor *store.BundleDescriptor, peers []cla.ConvergenceSender) {
	bundle, err := bundleDescriptor.Load()
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Failed to load bundle from disk")
		return
	}

	// Step 1: spawn a new goroutine for each cla
	currentPeers := cla.GetManagerSingleton().GetSenders()
	sentAtLeastOnce := false
	successfulSends := make([]bool, len(currentPeers))

	var wg sync.WaitGroup
	var once sync.Once

	wg.Add(len(currentPeers))
	for i, peer := range currentPeers {
		go func(peer cla.ConvergenceSender, i int) {
			log.WithFields(log.Fields{
				"bundle": bundleDescriptor.ID,
				"cla":    peer,
			}).Info("Sending bundle to a CLA (ConvergenceSender)")

			if err := peer.Send(bundle); err != nil {
				log.WithFields(log.Fields{
					"bundle": bundleDescriptor.ID,
					"cla":    peer,
					"error":  err,
				}).Warn("Sending bundle failed")
			} else {
				log.WithFields(log.Fields{
					"bundle": bundleDescriptor.ID,
					"cla":    peer,
				}).Debug("Sending bundle succeeded")

				successfulSends[i] = true

				once.Do(func() { sentAtLeastOnce = true })
			}

			wg.Done()
		}(peer, i)
	}
	wg.Wait()

	// Step 2 track which sends were successful
	for i, success := range successfulSends {
		if success {
			bundleDescriptor.AddAlreadySent(peers[i].GetPeerEndpointID())
		}
	}

	if sentAtLeastOnce {
		// TODO: send status report
	}
}

func DispatchPending() {
	bndls, err := store.GetStoreSingleton().GetDispatchable()
	if err != nil {
		log.WithError(err).Error("Error dispatching pending bundles")
	}

	for _, bndl := range bndls {
		err = BundleForwarding(bndl)
		if err != nil {
			log.WithFields(log.Fields{
				"error":  err,
				"bundle": bndl.ID,
			}).Error("Error forwarding bundle")
		}
	}
}
