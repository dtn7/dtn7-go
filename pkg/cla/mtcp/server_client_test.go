// SPDX-FileCopyrightText: 2019, 2020 Alvar Penning
// SPDX-FileCopyrightText: 2024 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package mtcp

import (
	"fmt"
	"net"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
)

func getRandomPort(t *rapid.T) int {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		t.Error(err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = l.Close() }()

	return l.Addr().(*net.TCPAddr).Port
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
		numberOfClients := rapid.IntRange(1, 25).Draw(t, "Number of Clients")
		numberOfBundles := rapid.IntRange(1, 1000).Draw(t, "Number of Bundles")
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
		serv := NewMTCPServer(
			fmt.Sprintf(":%d", port), bpv7.MustNewEndpointID("dtn://mtcpcla/"), receiveFunc)
		if err := serv.Start(); err != nil {
			t.Fatal(err)
		}

		clients := make([]*MTCPClient, numberOfClients)
		for i := 0; i < numberOfClients; i++ {
			client := NewAnonymousMTCPClient(fmt.Sprintf("localhost:%d", port))
			if err := client.Activate(); err != nil {
				t.Fatal(fmt.Errorf("starting Client failed: %v", err))
			}
			clients[i] = client
		}

		for i := 0; i < numberOfBundles; i++ {
			sender := clients[rapid.IntRange(0, len(clients)-1).Draw(t, fmt.Sprintf("Sender %v", i))]
			go func(i int, sender *MTCPClient) {
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
