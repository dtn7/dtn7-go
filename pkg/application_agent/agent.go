package application_agent

import (
	"fmt"
	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/store"
)

type ApplicationAgent interface {
	// Endpoints returns the EndpointIDs that this ApplicationAgent answers to.
	Endpoints() []bpv7.EndpointID

	Deliver(bundleDescriptor store.BundleDescriptor) error

	Shutdown()
}

type NoAgentRegisteredError bpv7.EndpointID

func NewNoAgentRegisteredError(eid bpv7.EndpointID) *NoAgentRegisteredError {
	err := NoAgentRegisteredError(eid)
	return &err
}

func (err *NoAgentRegisteredError) Error() string {
	return fmt.Sprintf("no agent registered for EndpointID %v", bpv7.EndpointID(*err))
}
