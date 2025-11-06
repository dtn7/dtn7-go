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

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
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

func setup() {
	receive := func(bundle *bpv7.Bundle) {}
	connect := func(eid bpv7.EndpointID) {}
	disconnect := func(eid bpv7.EndpointID) {}

	cla.InitialiseCLAManager(receive, connect, disconnect)
}

func teardown() {
	cla.GetManagerSingleton().Shutdown()
}

func TestSendReceive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		setup()
		defer teardown()

		port := getRandomPort(t)
		numberOfClients := rapid.IntRange(1, 25).Draw(t, "Number of Clients")
		numberOfBundles := uint8(rapid.IntRange(1, 256).Draw(t, "Number of Bundles"))
		var wgSend sync.WaitGroup
		wgSend.Add(int(numberOfBundles))
		var wgReceive sync.WaitGroup
		wgReceive.Add(int(numberOfBundles))

		bundles := make([]*bpv7.Bundle, numberOfBundles)
		var i uint8
		for i = 0; i < numberOfBundles; i++ {
			bundles[i] = bpv7.GenerateRandomizedBundle(t, i)
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

		for i = 0; i < numberOfBundles; i++ {
			sender := clients[rapid.IntRange(0, len(clients)-1).Draw(t, fmt.Sprintf("Sender %v", i))]
			go func(i uint8, sender *MTCPClient) {
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
