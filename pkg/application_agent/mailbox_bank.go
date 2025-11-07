package application_agent

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/store"
)

type MailboxBank struct {
	rwMutex sync.RWMutex

	registeredIDs []bpv7.EndpointID
	mailboxes     map[bpv7.EndpointID]*Mailbox
}

func NewMailboxBank() *MailboxBank {
	bank := MailboxBank{
		registeredIDs: make([]bpv7.EndpointID, 0),
		mailboxes:     make(map[bpv7.EndpointID]*Mailbox),
	}
	return &bank
}

func (bank *MailboxBank) Register(eid bpv7.EndpointID) error {
	bank.rwMutex.Lock()
	defer bank.rwMutex.Unlock()

	if _, ok := bank.mailboxes[eid]; ok {
		return NewIDAlreadyRegisteredError(eid)
	}

	bank.registeredIDs = append(bank.registeredIDs, eid)
	bank.mailboxes[eid] = NewMailbox()

	return nil
}

func (bank *MailboxBank) Unregister(eid bpv7.EndpointID) error {
	bank.rwMutex.Lock()
	defer bank.rwMutex.Unlock()

	if _, ok := bank.mailboxes[eid]; !ok {
		return NewNoSuchIDError(eid)
	}

	remainingIDs := make([]bpv7.EndpointID, 0, len(bank.registeredIDs))
	for _, reid := range bank.registeredIDs {
		if reid != eid {
			remainingIDs = append(remainingIDs, reid)
		}
	}
	bank.registeredIDs = remainingIDs

	delete(bank.mailboxes, eid)

	return nil
}

func (bank *MailboxBank) RegisteredIDs() []bpv7.EndpointID {
	bank.rwMutex.RLock()
	defer bank.rwMutex.RUnlock()

	return bank.registeredIDs
}

func (bank *MailboxBank) GetMailbox(eid bpv7.EndpointID) (*Mailbox, error) {
	bank.rwMutex.RLock()
	defer bank.rwMutex.RUnlock()

	mailbox, ok := bank.mailboxes[eid]
	if !ok {
		return nil, NewNoSuchIDError(eid)
	}

	return mailbox, nil
}

func (bank *MailboxBank) Deliver(bundleDescriptor *store.BundleDescriptor) error {
	bank.rwMutex.RLock()
	defer bank.rwMutex.RUnlock()

	destination := bundleDescriptor.Destination
	destinationMailbox, ok := bank.mailboxes[destination]
	if !ok {
		return NewNoSuchIDError(destination)
	}
	return destinationMailbox.Deliver(bundleDescriptor)
}

func (bank *MailboxBank) GC() {
	for eid, mailbox := range bank.mailboxes {
		log.WithField("eid", eid).Debug("Garbage collecting mailbox")
		go mailbox.GC()
	}
}
