package processing

import (
	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/application_agent"
	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/routing"
	"github.com/dtn7/dtn7-go/pkg/store"
)

func receiveAsync(bundle *bpv7.Bundle) {
	bundleDescriptor, err := store.GetStoreSingleton().InsertBundle(bundle)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundle.ID(),
			"error":  err,
		}).Error("Error storing new bundle")
		return
	}

	application_agent.GetManagerSingleton().Delivery(bundleDescriptor)

	routing.GetAlgorithmSingleton().NotifyNewBundle(bundleDescriptor)

	for _, constraint := range bundleDescriptor.RetentionConstraints {
		if constraint == store.DispatchPending {
			log.WithField("bundle", bundleDescriptor.IDString).Debug("Forwarding received bundle")
			BundleForwarding(bundleDescriptor)
		}
	}
}

func ReceiveBundle(bundle *bpv7.Bundle) {
	go receiveAsync(bundle)
}
