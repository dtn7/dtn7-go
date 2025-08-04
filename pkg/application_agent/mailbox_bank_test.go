package application_agent

import (
	"fmt"
	"math/rand"
	"testing"

	"pgregory.net/rapid"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

func initMailboxBankTest() *MailboxBank {
	bank := NewMailboxBank()
	return bank
}

func TestMailboxBank(t *testing.T) {
	rapid.Check(t, func(tr *rapid.T) {
		bank := initMailboxBankTest()

		nMailboxes := rapid.Uint8Min(1).Draw(tr, "Drawing number of mailboxes")
		eids := make([]bpv7.EndpointID, 0, nMailboxes)
		eidMap := make(map[bpv7.EndpointID]bool, nMailboxes)
		for i := range nMailboxes {
			eid, err := bpv7.NewEndpointID(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(tr, fmt.Sprintf("eid %v", i)))
			if err != nil {
				tr.Fatal(err)
			}

			if _, ok := eidMap[eid]; ok {
				continue
			}

			eids = append(eids, eid)
			eidMap[eid] = true

			err = bank.Register(eid)
			if err != nil {
				tr.Fatal(err)
			}
		}

		registered := bank.RegisteredIDs()
		for _, eid := range registered {
			if _, ok := eidMap[eid]; !ok {
				tr.Fatalf("MailboxBank has Mailbox for unregistered EndpointID: %v", eid)
			}
		}

		for eid := range eidMap {
			found := false
			for _, reid := range registered {
				if eid == reid {
					found = true
					break
				}
			}
			if !found {
				tr.Fatalf("EndpointID not in MailboxBank: %v", eid)
			}

			if _, err := bank.GetMailbox(eid); err != nil {
				tr.Fatalf("Error getting Mailbox for EndpointID %v: %v", eid, err)
			}
		}

		rIndex := rand.Intn(int(nMailboxes))
		remeid := eids[rIndex]

		if err := bank.Unregister(remeid); err != nil {
			tr.Fatalf("Error unregistering EndpointID %v: %v", remeid, err)
		}

		registered = bank.RegisteredIDs()
		for _, eid := range registered {
			if eid == remeid {
				tr.Fatalf("Removed EndpointID still in list of registered mailboxes: %v", remeid)
			}
		}

		if _, err := bank.GetMailbox(remeid); err == nil {
			tr.Fatalf("Removed EndpointID still returns mailbox: %v", remeid)
		}
	})
}
