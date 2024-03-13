package cla

import (
	"fmt"
	"testing"
	"time"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla/dummy_cla"
	log "github.com/sirupsen/logrus"
	"pgregory.net/rapid"
)

func setup(t *rapid.T) {
	receive := func(bundle *bpv7.Bundle) {}
	connect := func(eid bpv7.EndpointID) {}
	disconnect := func(eid bpv7.EndpointID) {}

	err := InitialiseCLAManager(receive, connect, disconnect)
	if err != nil {
		t.Fatal(err)
	}
}

func teardown() {
	GetManagerSingleton().Shutdown()
}

func TestRegisterListener(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup(t)
		defer teardown()

		numberOfListeners := rapid.Uint8Max(10).Draw(t, "Number of Listeners")
		listeners := make([]*dummy_cla.DummyListener, numberOfListeners)
		for i := 0; i < len(listeners); i++ {
			address := rapid.String().Draw(t, fmt.Sprintf("Listener %v address", i))
			listeners[i] = dummy_cla.NewDummyListener(address)
		}

		for _, listener := range listeners {
			err := GetManagerSingleton().RegisterListener(listener)
			if err != nil {
				t.Fatal(fmt.Sprintf("Error registering listener %v: %v", listener.Address(), err))
			}
		}

		initialisedListeners := GetManagerSingleton().GetListeners()
		for _, listener := range listeners {
			present := false
			for _, initialized := range initialisedListeners {
				if initialized.Address() == listener.Address() && initialized.Running() {
					present = true
					break
				}
			}
			if !present {
				t.Fatal(fmt.Sprintf("Listener with address %v not present", listener.Address()))
			}
		}
	})
}

func TestRegisterSender(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup(t)
		defer teardown()

		numberOfCLAs := rapid.Uint8Max(100).Draw(t, "Number of CLAs")
		clas := make([]Convergence, numberOfCLAs)

		noop := func(bundle bpv7.Bundle) (interface{}, error) {
			return nil, nil
		}

		for i := 0; i < len(clas); i++ {
			eid := bpv7.MustNewEndpointID(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(t, fmt.Sprintf("CLA %v", i)))
			cla, _ := dummy_cla.NewDummyCLAPair(eid, eid, noop)
			clas[i] = cla
			GetManagerSingleton().Register(cla)
		}

		time.Sleep(time.Millisecond * 10)

		for _, cla := range clas {
			senders := GetManagerSingleton().GetSenders()
			present := false
			for _, sender := range senders {
				if sender.Address() == cla.Address() {
					if sender.Active() {
						present = true
					} else {
						log.Fatalf("CLA %v not activated", cla.Address())
					}
				}
			}
			if !present {
				log.Fatalf("CLA %v not present", cla.Address())
			}
		}
	})
}
