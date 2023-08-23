package dummy_cla

import (
	"bytes"
	"fmt"
	"math/rand"
	"time"

	"github.com/dtn7/cboring"
	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	log "github.com/sirupsen/logrus"
)

// DummyCLA transfers bundles to another instance of DummyCLA via a channel
// Only used for testing
type DummyCLA struct {
	ownID           bpv7.EndpointID
	peerID          bpv7.EndpointID
	transferChannel chan []byte
	channelActive   bool
	receiveCallback func(bundle bpv7.Bundle) (interface{}, error)
}

func NewDummyCLAPair(peerAID bpv7.EndpointID, peerBID bpv7.EndpointID, receiveCallback func(bundle bpv7.Bundle) (interface{}, error)) (*DummyCLA, *DummyCLA) {
	transferChannel := make(chan []byte)
	peerA := DummyCLA{
		ownID:           peerAID,
		peerID:          peerBID,
		transferChannel: transferChannel,
		channelActive:   true,
		receiveCallback: receiveCallback,
	}
	peerB := DummyCLA{
		ownID:           peerBID,
		peerID:          peerAID,
		transferChannel: transferChannel,
		channelActive:   true,
		receiveCallback: receiveCallback,
	}
	return &peerA, &peerB
}

func (cla *DummyCLA) Close() error {
	wait := time.Duration(rand.Intn(10))
	time.Sleep(time.Millisecond * wait)
	if cla.channelActive {
		cla.channelActive = false
		close(cla.transferChannel)
	}
	return nil
}

func (cla *DummyCLA) Activate() error {
	go cla.handleReceive()
	return nil
}

func (cla *DummyCLA) Active() bool {
	return cla.channelActive
}

func (cla *DummyCLA) handleReceive() {
	for {
		bbytes, more := <-cla.transferChannel
		if !more {
			cla.channelActive = false
			return
		}
		serialiser := bytes.NewReader(bbytes)
		bundle := bpv7.Bundle{}
		err := cboring.Unmarshal(&bundle, serialiser)
		if err == nil {
			cla.receiveCallback(bundle)
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

func (cla *DummyCLA) Send(bundle bpv7.Bundle) error {
	var serialiser bytes.Buffer
	err := bundle.MarshalCbor(&serialiser)
	if err != nil {
		return err
	}
	bbytes := serialiser.Bytes()

	if !cla.channelActive {
		return fmt.Errorf("%v shut down", cla.Address())
	}

	cla.transferChannel <- bbytes

	return nil
}
