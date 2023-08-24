package cla

import (
	"sync"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla/dummy_cla"

	"github.com/dtn7/dtn7-ng/pkg/util"
	log "github.com/sirupsen/logrus"
)

// Manager keeps track of all active CLAs
type Manager struct {
	stateMutex sync.RWMutex
	receivers  []ConvergenceReceiver
	senders    []ConvergenceSender
	// pendingStart contains CLAs which are in the process of being started.
	// Since startup may fail, we don't directly add them to  the senders or receivers list
	pendingStart []Convergence
	// listeners run in the background and wait for incoming connections
	// They then need to spawn a new CLA and pass it to the manager's register-method
	listeners []ConvergenceListener

	// receiveCallback will be called for every received bundle
	// This is necessary since we can't directly import either the store or processing module without creating an import loop
	receiveCallback func(bundle *bpv7.Bundle)

	// connectCallback is called whenever a new peer connects.
	// This is necessary since we can't import the routing-module without creating an import loop
	connectCallback func(eid bpv7.EndpointID)

	// disconnectCallback is called whenever a new peer disconnects.
	// This is necessary since we can't import the routing-module without creating an import loop
	disconnectCallback func(eid bpv7.EndpointID)
}

// managerSingleton is the singleton object which should always be used for manager access
// We use this design pattern since there should ever only be a single manager
var managerSingleton *Manager

// InitialiseCLAManager initialises the manager-singleton
// To access Singleton-instance, use GetManagerSingleton
// Further calls to this function after initialisation will return a util.AlreadyInitialised-error
func InitialiseCLAManager(listeners []ListenerConfig, receiveCallback func(bundle *bpv7.Bundle), connectCallback func(eid bpv7.EndpointID), disconnectCallback func(eid bpv7.EndpointID)) error {
	if managerSingleton != nil {
		return util.NewAlreadyInitialisedError("CLA Manager")
	}

	manager := Manager{
		receivers:          make([]ConvergenceReceiver, 0, 10),
		senders:            make([]ConvergenceSender, 0, 10),
		pendingStart:       make([]Convergence, 0, 10),
		listeners:          make([]ConvergenceListener, 0, 10),
		receiveCallback:    receiveCallback,
		connectCallback:    connectCallback,
		disconnectCallback: disconnectCallback,
	}
	managerSingleton = &manager
	return managerSingleton.startListeners(listeners)
}

// GetManagerSingleton returns the manager singleton-instance.
// Attempting to call this function before manager initialisation will cause the program to panic.
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

// GetReceivers returns the list of currently active receiver-type CLAs
// This method is thread-safe
func (manager *Manager) GetReceivers() []ConvergenceReceiver {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()
	return manager.receivers
}

// GetListeners returns the list of CLA listeners
// This method is thread-safe
func (manager *Manager) GetListeners() []ConvergenceListener {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()
	return manager.listeners
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

	err := cla.Activate()
	if err != nil {
		log.WithFields(log.Fields{
			"cla":   cla.Address(),
			"error": err,
		}).Error("Failed to start CLA")
		// TODO: remove from pendingStart
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
	pending := make([]Convergence, 0, len(manager.pendingStart))
	for _, pendingCLA := range manager.pendingStart {
		if cla.Address() != pendingCLA.Address() {
			pending = append(pending, pendingCLA)
		}
	}
	manager.pendingStart = pending
}

// NotifyReceive is to be called by CLAs when they have received (and successfully unmarshalled) a bundle.
// This method spawns a new goroutine to handle the bundle asynchronously
func (manager *Manager) NotifyReceive(bundle *bpv7.Bundle) {
	go manager.receiveCallback(bundle)
}

// NotifyConnect is to be called by a CLA if it has successfully stared AND is a sender AND is aware of its neighbours EndpointID
// THis information is passed on to the routing algorithm asynchronously
func (manager *Manager) NotifyConnect(peerID bpv7.EndpointID) {
	go manager.connectCallback(peerID)
}

// NotifyDisconnect is to be called by a CLA if it notices that it has lost its connection
// Will remove the CLA from either or both of the manager's lists.
// This method is thread-safe.
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
		go manager.disconnectCallback(sender.GetPeerEndpointID())

		newSenders := make([]ConvergenceSender, len(manager.senders))
		for _, registeredSender := range manager.senders {
			if sender.Address() != registeredSender.Address() {
				newSenders = append(newSenders, registeredSender)
			}
		}
		manager.senders = newSenders
	}
}

func (manager *Manager) startListeners(listeners []ListenerConfig) error {
	for _, lst := range listeners {
		var listener ConvergenceListener
		switch lst.Type {
		case Dummy:
			listener = dummy_cla.NewDummyListener(lst.Address)
		default:
			return NewUnsupportedCLATypeError(lst.Type)
		}

		err := listener.Start()
		if err != nil {
			return err
		}

		manager.listeners = append(manager.listeners, listener)
	}
	return nil
}

func (manager *Manager) Shutdown() {
	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	for _, receiver := range manager.receivers {
		go receiver.Close()
	}
	manager.receivers = make([]ConvergenceReceiver, 0)

	for _, sender := range manager.senders {
		go sender.Close()
	}
	manager.senders = make([]ConvergenceSender, 0)

	for _, listener := range manager.listeners {
		go listener.Close()
	}
	manager.listeners = make([]ConvergenceListener, 0)

	managerSingleton = nil
}
