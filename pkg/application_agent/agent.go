// Package application_agent contains all application agents as well as the agent Manager.
// An application agent is any interface which allows users/applications to interact with the bundle node.
// The ApplicationAgent specifies which methods an application agent has to provide.
// Agents can be registered via the Manager's `Register` method.
// An agent can send bundles via the Manager's `Send` method
// Since there should only be a single agent Manager active at any time, this package employs the singleton pattern.
// Use `InitialiseApplicationAgentManager` and `GetManagerSingleton.`
package application_agent

import (
	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/store"
)

type ApplicationAgent interface {
	// Name is some string which uniquely identifies an agent.
	// Should include the agent type, as well as relevant config data (such as addresses that are listened on)
	Name() string

	// Endpoints returns the EndpointIDs that have been registered with this ApplicationAgent.
	Endpoints() []bpv7.EndpointID

	// Deliver delivers the bundle to  this Agent's mailboxes
	Deliver(bundleDescriptor *store.BundleDescriptor) error

	Start() error

	Shutdown()

	// GC performs garbage collection by removing references to stale bundles from mailboxes
	GC()
}

// bagContainsEndpoint checks if some bag/array/slice of endpoints contains another collection of endpoints.
func bagContainsEndpoint(bag []bpv7.EndpointID, eids []bpv7.EndpointID) bool {
	matches := map[bpv7.EndpointID]struct{}{}

	for _, eid := range eids {
		matches[eid] = struct{}{}
	}

	for _, eid := range bag {
		if _, ok := matches[eid]; ok {
			return true
		}
	}
	return false
}
