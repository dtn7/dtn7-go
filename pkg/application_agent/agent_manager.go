package application_agent

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/id_keeper"
	"github.com/dtn7/dtn7-ng/pkg/store"
)

type Manager struct {
	stateMutex   sync.RWMutex
	agents       []ApplicationAgent
	sendCallback func(bundle *bpv7.Bundle)
}

var managerSingleton *Manager

func InitialiseApplicationAgentManager(sendCallback func(bundle *bpv7.Bundle)) error {
	manager := Manager{
		agents:       make([]ApplicationAgent, 0, 10),
		sendCallback: sendCallback,
	}
	managerSingleton = &manager
	return nil
}

// GetManagerSingleton returns the manager singleton-instance.
// Attempting to call this function before store initialisation will cause the program to panic.
func GetManagerSingleton() *Manager {
	if managerSingleton == nil {
		log.Fatalf("Attempting to access an uninitialised agent manager. This must never happen!")
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

func (manager *Manager) UnregisterEndpoint(removeAgent ApplicationAgent) error {
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

func (manager *Manager) Delivery(bundleDescriptor *store.BundleDescriptor) {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()

	for _, agent := range manager.agents {
		err := agent.Deliver(bundleDescriptor)
		if err != nil {
			log.WithFields(log.Fields{
				"bundle": bundleDescriptor.ID,
				"agent":  agent,
				"error":  err,
			}).Error("Error delivering bundle")
		}
	}
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

func (manager *Manager) Send(bndl *bpv7.Bundle) {
	idKeeper := id_keeper.GetIdKeeperSingleton()
	idKeeper.Update(bndl)
	log.WithFields(log.Fields{"bundle": bndl.ID().String()}).Debug("Application agent sent bundle")
	manager.sendCallback(bndl)
}
