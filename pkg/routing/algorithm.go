package routing

import (
	"github.com/dtn7/dtn7-ng/pkg/cla"
	"github.com/dtn7/dtn7-ng/pkg/store"
)

type AlgorithmEnum int

const (
	Epidemic int = iota
)

// Algorithm is an interface to specify routing algorithms for delay-tolerant networks.
type Algorithm interface {
	// NotifyNewBundle notifies this Algorithm about new bundles. They
	// might be generated at this node or received from a peer. Whether an
	// algorithm acts on this information or ignores it, is an implementation matter.
	NotifyNewBundle(descriptor *store.BundleDescriptor)

	// SelectPeersForForwarding returns an array of ConvergenceSender for a requested bundle.
	// The CLA selection is based on the algorithm's design.
	SelectPeersForForwarding(descriptor *store.BundleDescriptor) (peers []cla.ConvergenceSender)

	// NotifyPeerAppeared notifies the Algorithm about a new peer.
	NotifyPeerAppeared(peer cla.Convergence)

	// NotifyPeerDisappeared notifies the Algorithm about the
	// disappearance of a peer.
	NotifyPeerDisappeared(peer cla.Convergence)
}

var AlgorithmSingleton Algorithm

func InitialiseAlgorithm(algorithm AlgorithmEnum) error {
	return nil
}
