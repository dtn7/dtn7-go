// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package dummy_cla provides a simple implementation of the CLA interface used for testing.
package dummy_cla

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/dtn7/cboring"
	"github.com/dtn7/dtn7-go/pkg/bpv7"
	log "github.com/sirupsen/logrus"
)

// DummyCLA transfers bundles to another instance of DummyCLA via a channel
// Only used for testing
type DummyCLA struct {
	ownID           bpv7.EndpointID
	peerID          bpv7.EndpointID
	transferChannel chan []byte
	channelActive   atomic.Bool
	receiveCallback func(bundle bpv7.Bundle) (interface{}, error)
}

func NewDummyCLAPair(peerAID bpv7.EndpointID, peerBID bpv7.EndpointID, receiveCallback func(bundle bpv7.Bundle) (interface{}, error)) (*DummyCLA, *DummyCLA) {
	transferChannel := make(chan []byte)
	peerA := DummyCLA{
		ownID:           peerAID,
		peerID:          peerBID,
		transferChannel: transferChannel,
		receiveCallback: receiveCallback,
	}
	peerB := DummyCLA{
		ownID:           peerBID,
		peerID:          peerAID,
		transferChannel: transferChannel,
		receiveCallback: receiveCallback,
	}
	return &peerA, &peerB
}

func (cla *DummyCLA) Close() error {
	wait := time.Duration(rand.Intn(10))
	time.Sleep(time.Millisecond * wait)
	active := cla.channelActive.Swap(false)
	if active {
		close(cla.transferChannel)
	}
	return nil
}

func (cla *DummyCLA) Activate() error {
	cla.channelActive.Store(true)
	go cla.handleReceive()
	return nil
}

func (cla *DummyCLA) Active() bool {
	return cla.channelActive.Load()
}

func (cla *DummyCLA) handleReceive() {
	for {
		bbytes, more := <-cla.transferChannel
		if !more {
			//cla.channelActive = false
			return
		}
		serialiser := bytes.NewReader(bbytes)
		bundle := bpv7.Bundle{}
		err := cboring.Unmarshal(&bundle, serialiser)
		if err == nil {
			_, err = cla.receiveCallback(bundle)
			if err != nil {
				log.WithFields(log.Fields{
					"cla":   cla.Address(),
					"error": err,
				}).Error("Error in receive-callback")
			}
		} else {
			log.WithFields(log.Fields{
				"cla":   cla.Address(),
				"error": err,
			}).Error("Error unmarshalling bundle")
		}
	}
}

func (cla *DummyCLA) Address() string {
	return fmt.Sprintf("dummycla://%v/", cla.ownID)
}

func (cla *DummyCLA) GetEndpointID() bpv7.EndpointID {
	return cla.ownID
}

func (cla *DummyCLA) GetPeerEndpointID() bpv7.EndpointID {
	return cla.peerID
}

func (cla *DummyCLA) Send(bundle *bpv7.Bundle) error {
	var serialiser bytes.Buffer
	err := bundle.MarshalCbor(&serialiser)
	if err != nil {
		return err
	}
	bbytes := serialiser.Bytes()

	if !cla.channelActive.Load() {
		return fmt.Errorf("%v shut down", cla.Address())
	}

	cla.transferChannel <- bbytes

	return nil
}
