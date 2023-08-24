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
	numberOfListeners := rapid.Uint8Max(10).Draw(t, "Number of Listeners")
	listeners := make([]ListenerConfig, numberOfListeners)
	for i := 0; i < len(listeners); i++ {
		listeners[i] = ListenerConfig{
			Address: rapid.String().Draw(t, fmt.Sprintf("Listener %v address", i)),
			Type:    Dummy,
		}
	}

	err := InitialiseCLAManager(listeners)
	if err != nil {
		t.Fatal(err)
	}

	initialisedListeners := GetManagerSingleton().GetListeners()
	for _, listenerConfig := range listeners {
		present := false
		for _, listener := range initialisedListeners {
			if listener.Address() == listenerConfig.Address && listener.Running() {
				present = true
				break
			}
		}
		if !present {
			t.Fatal(fmt.Sprintf("Listener with address %v not present", listenerConfig.Address))
		}
	}
}

func teardown() {
	GetManagerSingleton().Shutdown()
}

func TestRegister(t *testing.T) {
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
