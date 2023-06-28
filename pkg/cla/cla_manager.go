package cla

import "sync"

// CLAManager keeps track of all active CLAs
type CLAManager struct {
	stateMutex sync.RWMutex
	receivers  []ConvergenceReceiver
	senders    []ConvergenceSender
}

// CLAManagerSingleton is the singleton object which should always be used for manager access
// We use this design pattern since there should ever only be a single manager
var CLAManagerSingleton *CLAManager

func InitialiseCLAManager() {
	manager := CLAManager{
		receivers: make([]ConvergenceReceiver, 0, 10),
		senders:   make([]ConvergenceSender, 0, 10),
	}
	CLAManagerSingleton = &manager
}

func (manager *CLAManager) GetSenders() []ConvergenceSender {
	manager.stateMutex.RLock()
	defer manager.stateMutex.RUnlock()
	return manager.senders
}
