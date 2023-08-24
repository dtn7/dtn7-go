package processing

import (
	"errors"

	"github.com/dtn7/dtn7-ng/pkg/application_agent"
	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/routing"
	"github.com/dtn7/dtn7-ng/pkg/store"
	log "github.com/sirupsen/logrus"
)

func ReceiveBundle(bundle *bpv7.Bundle) {
	bundleDescriptor, err := store.GetStoreSingleton().InsertBundle(bundle)
	if err != nil {
		log.WithFields(log.Fields{
			"bundle": bundle.ID(),
			"error":  err,
		}).Error("Error storing new bundle")
		return
	}

	err = application_agent.GetManagerSingleton().Delivery(bundleDescriptor)
	if err != nil {
		var e *application_agent.NoAgentRegisteredError
		if errors.As(err, &e) {
			// this is actually normal
			log.WithFields(log.Fields{
				"bundle": bundleDescriptor.ID,
				"error":  err,
			}).Debug("No registered application agent for receiver ID")
		} else {
			log.WithFields(log.Fields{
				"bundle": bundleDescriptor.ID,
				"error":  err,
			}).Error("Error delivering bundle")
		}
	}

	routing.GetAlgorithmSingleton().NotifyNewBundle(bundleDescriptor)
}
