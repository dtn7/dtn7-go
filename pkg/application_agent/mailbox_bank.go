package application_agent

import (
	"fmt"
	"sync"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/store"
)

type IDAlreadyRegisteredError struct {
	eid bpv7.EndpointID
}

func NewIDAlreadyRegisteredError(eid bpv7.EndpointID) *IDAlreadyRegisteredError {
	err := IDAlreadyRegisteredError{
		eid: eid,
	}
	return &err
}

func (err *IDAlreadyRegisteredError) Error() string {
	return fmt.Sprintf("ID has already been registered: %v", err.eid.String())
}

type NoSuchIDError struct {
	eid bpv7.EndpointID
}

func NewNoSuchIDError(eid bpv7.EndpointID) *NoSuchIDError {
	err := NoSuchIDError{
		eid: eid,
	}
	return &err
}

func (err *NoSuchIDError) Error() string {
	return fmt.Sprintf("No such ID has been registered: %v", err.eid.String())
}

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
