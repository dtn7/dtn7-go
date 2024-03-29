// SPDX-FileCopyrightText: 2019, 2020 Alvar Penning
// SPDX-FileCopyrightText: 2022 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package quicl

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
)

func randomTcpPort(t *testing.T) (port int) {
	if addr, err := net.ResolveTCPAddr("tcp", "localhost:0"); err != nil {
		t.Fatal(err)
	} else if l, err := net.ListenTCP("tcp", addr); err != nil {
		t.Fatal(err)
	} else {
		port = l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
	}
	return
}

func randomData(size int) []byte {
	payload := make([]byte, size)

	rand.Seed(0)
	rand.Read(payload)

	return payload
}

func handleListener(
	listener cla.ConvergenceProvider, msgs, clients int, clientWg, serverWg *sync.WaitGroup, errs chan error,
) {
	var (
		msgsRecv  uint32
		msgsApprd uint32
	)

	defer serverWg.Done()

	manager := cla.NewManager()
	manager.Register(listener)

	go func() {
		for {
			switch cs := <-manager.Channel(); cs.MessageType {
			case cla.ReceivedBundle:
				log.Info("Accounted for bundle")
				atomic.AddUint32(&msgsRecv, 1)

			case cla.PeerAppeared:
				log.Info("Accounted for peer")
				atomic.AddUint32(&msgsApprd, 1)

				if sender, ok := cs.Sender.(cla.ConvergenceSender); !ok {
					errs <- fmt.Errorf("listener: new peer is not a ConvergenceSender; %v", cs)
				} else {
					log.Info("Sending response")
					dst := cs.Sender.(cla.ConvergenceSender).GetPeerEndpointID()
					bndl, err := bpv7.Builder().
						CRC(bpv7.CRC32).
						Source("dtn://server/").
						Destination(dst).
						CreationTimestampNow().
						Lifetime(30 * time.Minute).
						HopCountBlock(64).
						PayloadBlock([]byte("hello back!")).
						Build()
					if err != nil {
						errs <- fmt.Errorf("listener: %w", err)
					} else if err = sender.Send(bndl); err != nil {
						errs <- fmt.Errorf("listener for %v: %w", dst, err)
					}
				}
			}
		}
	}()

	clientWg.Wait()
	time.Sleep(time.Second)

	log.Info("Closing listener / manager")
	if err := manager.Close(); err != nil {
		errs <- err
	}

	if r := atomic.LoadUint32(&msgsRecv); r != uint32(msgs*clients) {
		errs <- fmt.Errorf("listener received %d messages instead of %d", r, msgs*clients)
	}
	if a := atomic.LoadUint32(&msgsApprd); a != uint32(clients) {
		errs <- fmt.Errorf("listener received %d appeared peers instead of %d", a, clients)
	}
}

func handleClient(
	mkClient func(string, bpv7.EndpointID) *Endpoint,
	serverAddr string, clientNo, msgs, payload int, clientWg *sync.WaitGroup, errs chan error,
) {
	defer clientWg.Done()

	log.WithField("client", clientNo).Info("Starting Client")

	clientEid := bpv7.MustNewEndpointID(fmt.Sprintf("dtn://client-%d/", clientNo))
	client := mkClient(serverAddr, clientEid)
	if err, _ := client.Start(); err != nil {
		errs <- fmt.Errorf("client %d: %w", clientNo, err)
		return
	}

	time.Sleep(time.Second)

	var thisClientWg sync.WaitGroup
	thisClientWg.Add(2)

	go func() {
		for {
			switch cs := <-client.Channel(); cs.MessageType {
			case cla.ReceivedBundle:
				log.WithField("client", clientNo).Info("Received bundle")
				thisClientWg.Done()
			case cla.PeerAppeared:
				log.WithField("client", clientNo).Info("Connected")
			}
		}
	}()

	go func() {
		defer thisClientWg.Done()

		for i := 0; i < msgs; i++ {
			bndl, err := bpv7.Builder().
				CRC(bpv7.CRC32).
				Source(clientEid).
				Destination("dtn://server/").
				CreationTimestampNow().
				Lifetime(30 * time.Minute).
				HopCountBlock(64).
				PayloadBlock(randomData(payload)).
				Build()

			if err != nil {
				errs <- fmt.Errorf("client %d: %w", clientNo, err)
			} else if err := client.Send(bndl); err != nil {
				errs <- fmt.Errorf("client %d: %w", clientNo, err)
			}
			log.WithFields(log.Fields{"client": clientNo, "bundle": i}).Info("Sent Bundle")
		}
	}()

	thisClientWg.Wait()
	time.Sleep(time.Second)

	log.WithField("client", clientNo).Info("Closing client")
	if err := client.Close(); err != nil {
		errs <- err
	}
}

func startTestNetwork(
	mkListener func(string) cla.ConvergenceProvider, mkClient func(string, bpv7.EndpointID) *Endpoint,
	msgs, clients, payload int, t *testing.T,
) {
	var serverAddr = fmt.Sprintf("localhost:%d", randomTcpPort(t))
	var errs = make(chan error)

	var clientWg sync.WaitGroup
	var serverWg sync.WaitGroup

	clientWg.Add(clients)
	serverWg.Add(1)

	go handleListener(mkListener(serverAddr), msgs, clients, &clientWg, &serverWg, errs)
	time.Sleep(250 * time.Millisecond)

	for i := 0; i < clients; i++ {
		go handleClient(mkClient, serverAddr, i, msgs, payload, &clientWg, errs)
	}

	go func() {
		serverWg.Wait()
		close(errs)
	}()

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestImplNetwork(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	mkQuiclListener := func(addr string) cla.ConvergenceProvider {
		return NewQUICListener(addr, bpv7.MustNewEndpointID("dtn://server/"))
	}
	mkQuiclDialer := func(addr string, eid bpv7.EndpointID) *Endpoint {
		return NewDialerEndpoint(addr, eid, false)
	}

	tests := []struct {
		protocol string

		clients int
		msgs    int
		payload int

		mkListener func(string) cla.ConvergenceProvider
		mkClient   func(string, bpv7.EndpointID) *Endpoint
	}{
		{"QUICL", 1, 1, 64, mkQuiclListener, mkQuiclDialer},
		{"QUICL", 1, 1, 2097152, mkQuiclListener, mkQuiclDialer},
		{"QUICL", 1, 256, 1024, mkQuiclListener, mkQuiclDialer},
		{"QUICL", 2, 1, 64, mkQuiclListener, mkQuiclDialer},
		{"QUICL", 2, 1, 2097152, mkQuiclListener, mkQuiclDialer},
		{"QUICL", 2, 256, 1024, mkQuiclListener, mkQuiclDialer},
		{"QUICL", 64, 1, 1024, mkQuiclListener, mkQuiclDialer},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s-%d-%d-%d", test.protocol, test.clients, test.msgs, test.payload), func(t *testing.T) {
			startTestNetwork(test.mkListener, test.mkClient, test.msgs, test.clients, test.payload, t)
		})
	}
}
