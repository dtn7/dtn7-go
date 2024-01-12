package application_agent

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/store"
	"github.com/hashicorp/go-multierror"
)

type Manager struct {
	stateMutex sync.RWMutex
	agents     []ApplicationAgent
}

var managerSingleton *Manager

func InitialiseApplicationAgentManager() error {
	manager := Manager{
		agents: make([]ApplicationAgent, 0, 10),
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

	endpoints := make([]bpv7.EndpointID, 0)

	for _, agent := range manager.agents {
		endpoints = append(endpoints, agent.Endpoints()...)
	}

	return endpoints
}

func (manager *Manager) RegisterAgent(newAgent ApplicationAgent) error {
	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	present := false
	for _, agent := range manager.agents {
		if agent == newAgent {
			present = true
			break
		}
	}

	if !present {
		manager.agents = append(manager.agents, newAgent)
	}

	// TODO: check if there are pending bundles for this endpoint ID

	return nil
}

func (manager *Manager) UnregisterEndpoint(id bpv7.EndpointID, removeAgent ApplicationAgent) error {
	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	remainingAgents := make([]ApplicationAgent, 0, len(manager.agents))
	for _, agent := range manager.agents {
		if agent != removeAgent {
			remainingAgents = append(remainingAgents, agent)
		}
	}

	manager.agents = remainingAgents
	return nil
}

func (manager *Manager) Delivery(bundleDescriptor *store.BundleDescriptor) error {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()

	var mErr error
	for _, agent := range manager.agents {
		err := agent.Deliver(bundleDescriptor)
		mErr = multierror.Append(mErr, err)
	}

	return mErr
}

func (manager *Manager) Shutdown() {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()

	for _, agent := range manager.agents {
		agent.Shutdown()
	}

	manager.agents = make([]ApplicationAgent, 0)

	managerSingleton = nil
}

func (manager *Manager) Send(bndl *bpv7.Bundle) error {
	_, err := store.GetStoreSingleton().InsertBundle(bndl)
	return err
}
