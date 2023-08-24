package application_agent

import (
	"sync"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/store"
	"github.com/hashicorp/go-multierror"
	log "github.com/sirupsen/logrus"
)

type Manager struct {
	stateMutex sync.RWMutex
	agents     []ApplicationAgent
	endpoints  map[bpv7.EndpointID][]ApplicationAgent
}

var managerSingleton *Manager

func InitialiseApplicationAgentManager() error {
	manager := Manager{
		agents:    make([]ApplicationAgent, 0, 10),
		endpoints: make(map[bpv7.EndpointID][]ApplicationAgent),
	}
	managerSingleton = &manager
	return nil
}

// GetManagerSingleton returns the manager singleton-instance.
// Attempting to call this function before store initialisation will cause the program to panic.
func GetManagerSingleton() *Manager {
	if managerSingleton == nil {
		log.Fatalf("Attempting to access an uninitialised manager. This must never happen!")
	}
	return managerSingleton
}

// GetEndpoints returns a slice of all registered Endpoints on this node
func (manager *Manager) GetEndpoints() []bpv7.EndpointID {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()

	endpoints := make([]bpv7.EndpointID, len(manager.endpoints))
	i := 0
	for eid := range manager.endpoints {
		endpoints[i] = eid
		i++
	}

	return endpoints
}

func (manager *Manager) RegisterEndpoint(newID bpv7.EndpointID, newAgent ApplicationAgent) error {
	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	agents, exists := manager.endpoints[newID]
	present := false
	if !exists {
		agents = make([]ApplicationAgent, 0, 1)
	} else {
		for _, agent := range agents {
			if agent == newAgent {
				present = true
				break
			}
		}
	}

	if !present {
		agents = append(agents, newAgent)
		manager.endpoints[newID] = agents
	}

	// TODO: check if there are pending bundles for this endpoint ID

	return nil
}

func (manager *Manager) UnregisterEndpoint(id bpv7.EndpointID, removeAgent ApplicationAgent) error {
	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	agents, exists := manager.endpoints[id]
	if !exists {
		return NewNoAgentRegisteredError(id)
	}

	remainingAgents := make([]ApplicationAgent, 0, len(agents))
	for _, agent := range agents {
		if agent != removeAgent {
			remainingAgents = append(remainingAgents, agent)
		}
	}

	manager.endpoints[id] = remainingAgents
	return nil
}

func (manager *Manager) Delivery(bundleDescriptor *store.BundleDescriptor) error {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()

	agents, exists := manager.endpoints[bundleDescriptor.Destination]
	if !exists {
		return NewNoAgentRegisteredError(bundleDescriptor.Destination)
	}

	var mErr error
	for _, agent := range agents {
		err := agent.Deliver(bundleDescriptor)
		mErr = multierror.Append(mErr, err)
	}

	return mErr
}
