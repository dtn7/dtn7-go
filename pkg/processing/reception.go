package processing

import (
	"github.com/dtn7/dtn7-ng/pkg/application_agent"
	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/routing"
	"github.com/dtn7/dtn7-ng/pkg/store"
	log "github.com/sirupsen/logrus"
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

	err = BundleForwarding(bundleDescriptor)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundleDescriptor.ID,
			"error":  err,
		}).Error("Error forwarding bundle")
		return
	}
}

func ReceiveBundle(bundle *bpv7.Bundle) {
	go receiveAsync(bundle)
}
