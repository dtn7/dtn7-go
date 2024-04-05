package quicl

import (
	"fmt"
	"net"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
)

const (
	maxClients = 1
	maxBundles = 1
)

func getRandomPort(t *rapid.T) int {
	addr, err := net.ResolveUDPAddr("udp", "localhost:0")
	if err != nil {
		t.Error(err)
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = l.Close() }()

	return l.LocalAddr().(*net.UDPAddr).Port
}

func setup(t *rapid.T) {
	receive := func(bundle *bpv7.Bundle) {}
	connect := func(eid bpv7.EndpointID) {}
	disconnect := func(eid bpv7.EndpointID) {}

	err := cla.InitialiseCLAManager(receive, connect, disconnect)
	if err != nil {
		t.Fatal(err)
	}
}

func teardown() {
	cla.GetManagerSingleton().Shutdown()
}

func TestSendReceive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup(t)
		defer teardown()

		port := getRandomPort(t)
		numberOfClients := rapid.IntRange(1, maxClients).Draw(t, "Number of Clients")
		numberOfBundles := rapid.IntRange(1, maxBundles).Draw(t, "Number of Bundles")
		var wgSend sync.WaitGroup
		wgSend.Add(numberOfBundles)
		var wgReceive sync.WaitGroup
		wgReceive.Add(numberOfBundles)

		bundles := make([]bpv7.Bundle, numberOfBundles)
		for i := 0; i < numberOfBundles; i++ {
			bundles[i] = bpv7.GenerateBundle(t, i)
		}

		receiveFunc := func(bundle *bpv7.Bundle) {
			wgReceive.Done()
		}

		// Server
		serv := NewQUICListener(
			fmt.Sprintf(":%d", port), bpv7.MustNewEndpointID("dtn://quicl/"), receiveFunc)
		if err := serv.Start(); err != nil {
			t.Fatal(err)
		}

		clients := make([]*Endpoint, numberOfClients)
		for i := 0; i < numberOfClients; i++ {
			client := NewDialerEndpoint(fmt.Sprintf("localhost:%d", port), bpv7.DtnNone(), receiveFunc)
			if err := client.Activate(); err != nil {
				t.Fatal(fmt.Errorf("starting Client failed: %v", err))
			}
			clients[i] = client
		}

		for i := 0; i < numberOfBundles; i++ {
			sender := clients[rapid.IntRange(0, len(clients)-1).Draw(t, fmt.Sprintf("Sender %v", i))]
			go func(i int, sender *Endpoint) {
				bundle := bundles[i]
				err := sender.Send(bundle)
				wgSend.Done()
				if err != nil {
					t.Fatal(err)
				}
			}(i, sender)
		}
		wgSend.Wait()
		wgReceive.Wait()

		for _, client := range clients {
			err := client.Close()
			if err != nil {
				t.Fatal(err)
			}
		}

		err := serv.Close()
		if err != nil {
			t.Fatal(err)
		}
	})
}
