package cla

import (
	"sync"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
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

	disconnectMutex sync.Mutex
	pendingRemoval  map[string]bool

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
// Further calls to this function after initialisation will panic.
func InitialiseCLAManager(receiveCallback func(bundle *bpv7.Bundle), connectCallback func(eid bpv7.EndpointID), disconnectCallback func(eid bpv7.EndpointID)) {
	if managerSingleton != nil {
		log.Fatalf("Attempting to access an uninitialised CLA manager. This must never happen!")
	}

	manager := Manager{
		receivers:          make([]ConvergenceReceiver, 0, 10),
		senders:            make([]ConvergenceSender, 0, 10),
		pendingStart:       make([]Convergence, 0, 10),
		listeners:          make([]ConvergenceListener, 0, 10),
		receiveCallback:    receiveCallback,
		connectCallback:    connectCallback,
		disconnectCallback: disconnectCallback,
		pendingRemoval:     make(map[string]bool),
	}
	managerSingleton = &manager
}

// GetManagerSingleton returns the manager singleton-instance.
// Attempting to call this function before manager initialisation will cause the program to panic.
func GetManagerSingleton() *Manager {
	if managerSingleton == nil {
		log.Fatalf("Attempting to access an uninitialised CLA manager. This must never happen!")
	}
	return managerSingleton
}

func ShutdownCLAManager() {
	if managerSingleton == nil {
		return
	}
	managerSingleton.shutdown()
}

func (manager *Manager) shutdown() {
	managerSingleton = nil

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

// checkPresent checks if a CLA has been registered with the manager
// This method is NOT threadsafe - you must have locked the stateMutex BEFORE calling this.
func (manager *Manager) checkPresent(cla Convergence) bool {
	// check if this CLA is present in the manager's pendingStart-list
	for _, pending := range manager.pendingStart {
		if cla.Address() == pending.Address() {
			log.WithField("cla", cla.Address()).Debug("CLA already registered as pending")
			return true
		}
	}

	// check if this CLA is present in the manager's receiver-list
	if _, ok := cla.(ConvergenceReceiver); ok {
		for _, registeredReceiver := range manager.receivers {
			if cla.Address() == registeredReceiver.Address() {
				log.WithField("cla", cla.Address()).Debug("CLA already registered as receiver")
				return true
			}
		}
	}

	// check if this CLA is present in the manager's sender-list
	if _, ok := cla.(ConvergenceSender); ok {
		for _, registeredSender := range manager.senders {
			if cla.Address() == registeredSender.Address() {
				log.WithField("cla", cla.Address()).Debug("CLA already registered as sender")
				return true
			}
		}
	}

	return false
}

// CheckPresent checks if a CLA has been registered with the manager
func (manager *Manager) CheckPresent(cla Convergence) bool {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()

	return manager.checkPresent(cla)
}

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
	defer manager.stateMutex.Unlock()

	present := manager.checkPresent(cla)
	if present {
		return
	}

	// add CLA to pendingStart, so that no-one else will try to start it while we're still working
	manager.pendingStart = append(manager.pendingStart, cla)
	log.WithField("cla", cla.Address()).Debug("Added cla to pending")

	go manager.activateCLA(cla)
}

// activateCLA is spawned by registerAsync to actually activate the CLA.
// Will call activate on CLA. Once Call returns, we remove the CLA from pending, and if activation was successful, add it to the registered CLAs.
func (manager *Manager) activateCLA(cla Convergence) {
	log.WithField("cla", cla.Address()).Debug("Activating CLA")

	err := cla.Activate()
	if err != nil {
		log.WithFields(log.Fields{
			"cla":   cla.Address(),
			"error": err,
		}).Error("Failure activating CLA")
	} else {
		log.WithField("cla", cla.Address()).Debug("CLA activated successfully")
	}

	manager.stateMutex.Lock()
	defer manager.stateMutex.Unlock()

	// remove the cla from the pending-list
	pending := make([]Convergence, 0, len(manager.pendingStart))
	for _, pendingCLA := range manager.pendingStart {
		if cla.Address() != pendingCLA.Address() {
			pending = append(pending, pendingCLA)
		}
	}
	manager.pendingStart = pending
	log.WithField("cla", cla).Debug("CLA removed from pending")

	if err == nil {
		// add the CLA to the corresponding lists
		// Note that a single object can be both a sender and receiver
		if receiver, ok := cla.(ConvergenceReceiver); ok {
			manager.receivers = append(manager.receivers, receiver)
			log.WithField("cla", cla).Debug("CLA added to receivers")
		}
		if sender, ok := cla.(ConvergenceSender); ok {
			manager.senders = append(manager.senders, sender)
			log.WithField("cla", cla).Debug("CLA added to senders")
		}
	}
}

// NotifyReceive is to be called by CLAs when they have received (and successfully unmarshalled) a bundle.
// This method spawns a new goroutine to handle the bundle asynchronously
func (manager *Manager) NotifyReceive(bundle *bpv7.Bundle) {
	log.WithField("bundle", bundle.ID().String()).Debug("Received bundle")
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
	log.WithField("cla", cla).Info("CLA disappeared")

	manager.disconnectMutex.Lock()
	_, present := manager.pendingRemoval[cla.Address()]
	if present {
		log.WithField("cla", cla).Debug("CLA already pending removal")
		manager.disconnectMutex.Unlock()
		return
	}
	manager.pendingRemoval[cla.Address()] = true
	manager.disconnectMutex.Unlock()

	manager.stateMutex.Lock()
	log.WithField("cla", cla).Debug("Acquired state lock")
	defer log.WithField("cla", cla).Debug("Released state lock")
	defer manager.stateMutex.Unlock()

	if disappearedReceiver, ok := cla.(ConvergenceReceiver); ok {
		log.WithField("cla", cla).Debug("CLA was receiver")
		newReceivers := make([]ConvergenceReceiver, 0, len(manager.receivers))
		for _, registeredReceiver := range manager.receivers {
			if disappearedReceiver.Address() != registeredReceiver.Address() {
				newReceivers = append(newReceivers, registeredReceiver)
			}
		}
		log.WithFields(log.Fields{
			"cla":                 cla,
			"remaining receivers": newReceivers,
		}).Debug("Receivers remaining after filter")
		manager.receivers = newReceivers
	}

	if sender, ok := cla.(ConvergenceSender); ok {
		log.WithField("cla", cla).Debug("CLA was sender")
		go manager.disconnectCallback(sender.GetPeerEndpointID())

		newSenders := make([]ConvergenceSender, 0, len(manager.senders))
		for _, registeredSender := range manager.senders {
			if sender.Address() != registeredSender.Address() {
				newSenders = append(newSenders, registeredSender)
			}
		}
		log.WithFields(log.Fields{
			"cla":               cla,
			"remaining senders": newSenders,
		}).Debug("Senders remaining after filter")
		manager.senders = newSenders
	}

	manager.disconnectMutex.Lock()
	defer manager.disconnectMutex.Unlock()
	delete(manager.pendingRemoval, cla.Address())
}

// RegisterListener starts the ConvergenceListener
// and, if the startup succeeds, adds it to the Manager's list of listeners
func (manager *Manager) RegisterListener(listener ConvergenceListener) error {
	err := listener.Start()
	if err != nil {
		return err
	}

	manager.listeners = append(manager.listeners, listener)

	return nil
}

// GetFilteredPeers returns a slice of ConvergenceSenders which connect to nodes that are not in the given
// list of EndpointIDs, i.e. nodes that are not known to already hold the bundle.
func (manager *Manager) GetFilteredPeers(knownHolders []bpv7.EndpointID) []ConvergenceSender {
	return filterPeers(knownHolders, manager.GetSenders())
}

// filterPeers filters out nodes which are contained in knownHolders, i.e. nodes which already received a Bundle.
// It returns a list of unused ConvergenceSenders.
func filterPeers(knownHolders []bpv7.EndpointID, clas []ConvergenceSender) (filtered []ConvergenceSender) {
	filtered = make([]ConvergenceSender, 0, len(clas))

	for _, cs := range clas {
		skip := false

		for _, eid := range knownHolders {
			if cs.GetPeerEndpointID() == eid {
				skip = true
				break
			}
		}

		if !skip {
			filtered = append(filtered, cs)
		}
	}

	return
}
