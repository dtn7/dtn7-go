// SPDX-FileCopyrightText: 2019, 2020, 2021 Alvar Penning
//
// SPDX-License-Identifier: GPL-3.0-or-later

package mtcp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/cboring"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

// MTCPServer is an implementation of a Minimal TCP Convergence-Layer server
// which accepts bundles from multiple connections and forwards them to the
// given channel. This struct implements a ConvergenceReceiver.
type MTCPServer struct {
	listenAddress string
	endpointID    bpv7.EndpointID
	running       bool

	receiveCallback func(*bpv7.Bundle)

	stopSyn chan struct{}
	stopAck chan struct{}
}

// NewMTCPServer creates a new MTCPServer for the given listen address. The
// permanent flag indicates if this MTCPServer should never be removed from
// the core.
func NewMTCPServer(listenAddress string, endpointID bpv7.EndpointID, receiveCallback func(*bpv7.Bundle)) *MTCPServer {
	return &MTCPServer{
		listenAddress:   listenAddress,
		endpointID:      endpointID,
		running:         false,
		receiveCallback: receiveCallback,
		stopSyn:         make(chan struct{}),
		stopAck:         make(chan struct{}),
	}
}

func (serv *MTCPServer) Start() error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", serv.listenAddress)
	if err != nil {
		return err
	}

	ln, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}

	go func(ln *net.TCPListener) {
		for {
			select {
			case <-serv.stopSyn:
				serv.running = false
				_ = ln.Close()
				close(serv.stopAck)

				return

			default:
				if err := ln.SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
					log.WithFields(log.Fields{
						"cla":   serv,
						"error": err,
					}).Error("MTCPServer failed to set deadline on TCP socket")

					_ = serv.Close()
				} else if conn, err := ln.Accept(); err == nil {
					go serv.handleSender(conn)
				}
			}
		}
	}(ln)

	serv.running = true

	return nil
}

func (serv *MTCPServer) handleSender(conn net.Conn) {
	defer func() {
		_ = conn.Close()

		if r := recover(); r != nil {
			log.WithFields(log.Fields{
				"cla":   serv,
				"conn":  conn,
				"error": r,
			}).Error("MTCPServer's sender failed")
		}
	}()

	log.WithFields(log.Fields{
		"cla":  serv,
		"conn": conn,
	}).Debug("MTCP handleServer connection was established")

	connReader := bufio.NewReader(conn)
	for {
		if n, err := cboring.ReadByteStringLen(connReader); err != nil {
			if err != io.EOF {
				log.WithFields(log.Fields{
					"cla":   serv,
					"conn":  conn,
					"error": err,
				}).Warn("MTCP handleServer connection failed to read byte string len")
			}

			// There is no use in sending an PeerDisappeared Message at this point,
			// because a MTCPServer might hold multiple clients. Furthermore, there
			// is no linkage between unknown connections and Endpoint IDs.

			return
		} else if n == 0 {
			continue
		}

		bndl := new(bpv7.Bundle)
		if err := cboring.Unmarshal(bndl, connReader); err != nil {
			log.WithFields(log.Fields{
				"cla":   serv,
				"conn":  conn,
				"error": err,
			}).Error("MTCP handleServer connection failed to read bundle")

			return
		} else {
			log.WithFields(log.Fields{
				"cla":  serv,
				"conn": conn,
			}).Debug("MTCP handleServer connection received a bundle")

			serv.receiveCallback(bndl)
		}
	}
}

func (serv *MTCPServer) Close() error {
	close(serv.stopSyn)
	<-serv.stopAck

	return nil
}

func (serv *MTCPServer) GetEndpointID() bpv7.EndpointID {
	return serv.endpointID
}

func (serv *MTCPServer) Address() string {
	return fmt.Sprintf("mtcp://%s", serv.listenAddress)
}

func (serv *MTCPServer) String() string {
	return serv.Address()
}

func (serv *MTCPServer) Active() bool {
	return serv.running
}

func (serv *MTCPServer) Activate() error {
	return nil
}

func (serv *MTCPServer) Running() bool {
	return serv.running
}
