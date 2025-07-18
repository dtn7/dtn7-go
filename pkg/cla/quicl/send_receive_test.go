package quicl

import (
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"pgregory.net/rapid"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
)

const (
	testTimeout = "10s"
	maxClients  = 10
	maxBundles  = 10
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

func setup(t *testing.T) {
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

func TestSingle(t *testing.T) {
	setup(t)
	defer teardown()

	timeout, err := time.ParseDuration(testTimeout)
	if err != nil {
		t.Fatalf("Error parsing timeout: %v", err.Error())
	}

	bndl := bpv7.GenerateSampleBundle(t)
	port := 35037
	recvChan := make(chan *bpv7.Bundle)

	receiveFunc := func(bundle *bpv7.Bundle) {
		log.WithField("bundle", bundle).Debug("Received bundle")
		recvChan <- bundle
	}

	// Server
	serv := NewQUICListener(
		fmt.Sprintf(":%d", port), bpv7.MustNewEndpointID("dtn://quicl/"), receiveFunc)
	if err := serv.Start(); err != nil {
		t.Fatal(err)
	}

	client := NewDialerEndpoint(fmt.Sprintf("localhost:%d", port), bpv7.DtnNone(), receiveFunc)
	if err := client.Activate(); err != nil {
		t.Fatal(fmt.Errorf("starting Client failed: %v", err))
	}
	err = client.Send(bndl)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case bndlRecv := <-recvChan:
		if !reflect.DeepEqual(*bndl, *bndlRecv) {
			t.Fatalf("Bundle changed during transmission: %v, %v", bndl, bndlRecv)
		}
	case <-time.After(timeout):
		t.Fatalf("Test timed out") // timed out
	}
}

// FIXME: the waitgroup-thing is crap, since if the test fails, it just hangs forever. Need to find a better way...
/*
func TestProperty(t *testing.T) {
	setup(t)
	defer teardown()
	rapid.Check(t, func(t *rapid.T) {

		port := getRandomPort(t)
		numberOfClients := rapid.IntRange(1, maxClients).Draw(t, "Number of Clients")
		numberOfBundles := rapid.IntRange(1, maxBundles).Draw(t, "Number of Bundles")
		var wgSend sync.WaitGroup
		wgSend.Add(numberOfBundles)
		var wgReceive sync.WaitGroup
		wgReceive.Add(numberOfBundles)

		bundles := make([]bpv7.Bundle, numberOfBundles)
		for i := 0; i < numberOfBundles; i++ {
			bundles[i] = bpv7.GenerateRandomizedBundle(t, i)
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
*/
