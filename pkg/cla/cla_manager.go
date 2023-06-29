package cla

import (
	"sync"

	"github.com/dtn7/dtn7-ng/pkg/util"
	log "github.com/sirupsen/logrus"
)

// Manager keeps track of all active CLAs
type Manager struct {
	stateMutex   sync.RWMutex
	receivers    []ConvergenceReceiver
	senders      []ConvergenceSender
	pendingStart []Convergence
}

// managerSingleton is the singleton object which should always be used for manager access
// We use this design pattern since there should ever only be a single manager
var managerSingleton *Manager

// InitialiseCLAManager initialises the manager-singleton
// To access Singleton-instance, use GetManagerSingleton
// Further calls to this function after initialisation will return a util.AlreadyInitialised-error
func InitialiseCLAManager() error {
	if managerSingleton != nil {
		return util.NewAlreadyInitialisedError("CLA Manager")
	}

	manager := Manager{
		receivers:    make([]ConvergenceReceiver, 0, 10),
		senders:      make([]ConvergenceSender, 0, 10),
		pendingStart: make([]Convergence, 0, 10),
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

// GetSenders returns the list of currently active sender-type CLAs
// This method is thread-safe
func (manager *Manager) GetSenders() []ConvergenceSender {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()
	return manager.senders
}

// TODO: Method to create CLA from parameters

// Register is the exported method to register a new CLA.
// All it does is spawn the actual registration in a goroutine and return immediately
// This is done to avoid deadlocks where another process may indefinitely wait for the CLA's
// Start-method to return
// This method is thread-safe
func (manager *Manager) Register(cla Convergence) {
	go manager.registerAsync(cla)
}

// registerAsync performs the actual CLA registration
// It will call the CLA's Start-method, wait for it to return and if no error was produced,
// the CLA will be added to the manager's sender/receiver lists.
func (manager *Manager) registerAsync(cla Convergence) {
	manager.stateMutex.Lock()
	// check if this CLA is present in the manager's pendingStart-list
	for _, pending := range manager.pendingStart {
		if cla.Address() == pending.Address() {
			log.WithField("cla", cla.Address()).Debug("CLA already being started")
			manager.stateMutex.Unlock()
			return
		}
	}

	// check if this CLA is present in the manager's receiver-list
	if _, ok := cla.(ConvergenceReceiver); ok {
		for _, registerdReceiver := range manager.receivers {
			if cla.Address() == registerdReceiver.Address() {
				log.WithField("cla", cla.Address()).Debug("CLA already registered as receiver")
				manager.stateMutex.Unlock()
				return
			}
		}
	}

	// check if this CLA is present in the manager's sender-list
	if _, ok := cla.(ConvergenceSender); ok {
		for _, registeredSender := range manager.senders {
			if cla.Address() == registeredSender.Address() {
				log.WithField("cla", cla.Address()).Debug("CLA already registered as sender")
				manager.stateMutex.Unlock()
				return
			}
		}
	}

	// add CLA to pendingStart, so that no-one else will try to start it while we're still working
	manager.pendingStart = append(manager.pendingStart, cla)
	manager.stateMutex.Unlock()

	err := cla.Start()
	if err != nil {
		log.WithFields(log.Fields{
			"cla":   cla.Address(),
			"error": err,
		}).Error("Failed to start CLA")
		return
	}

	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	// add the CLA to the corresponding lists
	// Note that a single object can be both a sender and receiver
	if receiver, ok := cla.(ConvergenceReceiver); ok {
		manager.receivers = append(manager.receivers, receiver)
	}
	if sender, ok := cla.(ConvergenceSender); ok {
		manager.senders = append(manager.senders, sender)
	}

	// remove the cla from the pending-list
	pending := make([]Convergence, len(manager.pendingStart))
	for _, pendingCLA := range manager.pendingStart {
		if cla.Address() != pendingCLA.Address() {
			pending = append(pending, pendingCLA)
		}
	}
	manager.pendingStart = pending
}

// NotifyDisconnect is to be called by a CLA if it notices that it has lost its connection
// Will remove the CLA from either or both of the manager's lists
// This method is thread-safe
func (manager *Manager) NotifyDisconnect(cla Convergence) {
	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	if receiver, ok := cla.(ConvergenceReceiver); ok {
		newReceivers := make([]ConvergenceReceiver, len(manager.receivers))
		for _, registeredReceiver := range manager.receivers {
			if receiver.Address() != registeredReceiver.Address() {
				newReceivers = append(newReceivers, registeredReceiver)
			}
		}
		manager.receivers = newReceivers
	}

	if sender, ok := cla.(ConvergenceSender); ok {
		newSenders := make([]ConvergenceSender, len(manager.senders))
		for _, registeredSender := range manager.senders {
			if sender.Address() != registeredSender.Address() {
				newSenders = append(newSenders, registeredSender)
			}
		}
		manager.senders = newSenders
	}
}
