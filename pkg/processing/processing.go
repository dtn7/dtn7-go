package processing

import (
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
	bundleDescriptor.AddConstraint(store.ForwardPending)
	bundleDescriptor.RemoveConstraint(store.DispatchPending)

	// Step 2: determine if contraindicated - whatever that means
	// Step 2.1: Call routing algorithm(?)
	forwardToPeers := routing.ActiveAlgorithm.SelectPeersForForwarding(bundleDescriptor)

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

	// TODO: Step 5: generate a bunch of status reports

	// Step 6: remove "Forward Pending"
	bundleDescriptor.RemoveConstraint(store.ForwardPending)
	return nil
}

func bundleContraindicated(bundleDescriptor *store.BundleDescriptor) error {
	return nil
}

func forwardBundle(bundleDescriptor *store.BundleDescriptor, peers []cla.ConvergenceSender) {

}
