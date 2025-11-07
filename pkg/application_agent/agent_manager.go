// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package application_agent

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/id_keeper"
	"github.com/dtn7/dtn7-go/pkg/store"
)

type Manager struct {
	stateMutex   sync.RWMutex
	agents       map[string]ApplicationAgent
	sendCallback func(bundle *bpv7.Bundle)
}

var managerSingleton *Manager

func InitialiseApplicationAgentManager(sendCallback func(bundle *bpv7.Bundle)) error {
	manager := Manager{
		agents:       make(map[string]ApplicationAgent),
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

func (manager *Manager) Shutdown() {
	managerSingleton = nil

	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()

	for agentName, agent := range manager.agents {
		delete(manager.agents, agentName)
		go agent.Shutdown()
	}
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

// RegisterAgent registers and start a new ApplicationAgent
// If an agent with the same name is already registered, then method returns an AgentAlreadyRegisteredError
// If the agent's startup fails,the resulting error will be returned, and the agent will NOT be registered.
func (manager *Manager) RegisterAgent(newAgent ApplicationAgent) error {
	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	agentName := newAgent.Name()

	if _, ok := manager.agents[agentName]; ok {
		return NewAgentAlreadyRegisteredError(agentName)
	}

	err := newAgent.Start()
	if err != nil {
		return err
	}

	manager.agents[agentName] = newAgent

	return nil
}

// UnregisterAgent stops an application agent and removes it from the manager.
// If no agent with the given name is registered, then method returns a NoSuchAgentError.
func (manager *Manager) UnregisterAgent(agentName string) error {
	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	agent, ok := manager.agents[agentName]
	if !ok {
		return NewNoSuchAgentError(agentName)
	}

	delete(manager.agents, agentName)
	agent.Shutdown()

	return nil
}

// Delivery attempts to deliver a bundle to all registered agents
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
			}).Debug("Error delivering bundle")
		}
	}
}

// Send is a callback to be used by agents to send a newly created bundle
func (manager *Manager) Send(bndl *bpv7.Bundle) {
	idKeeper := id_keeper.GetIdKeeperSingleton()
	idKeeper.Update(bndl)
	log.WithFields(log.Fields{"bundle": bndl.ID().String()}).Debug("Application agent sent bundle")
	manager.sendCallback(bndl)
}
