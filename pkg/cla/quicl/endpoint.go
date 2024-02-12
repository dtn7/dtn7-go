// SPDX-FileCopyrightText: 2022 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package quicl

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/dtn7/cboring"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
	"github.com/dtn7/dtn7-ng/pkg/cla/quicl/internal"
	"github.com/quic-go/quic-go"
	log "github.com/sirupsen/logrus"
)

// TODO: is this a reasonable value? I don't know...
const handshakeTimeout = 500 * time.Millisecond

type Endpoint struct {
	// id is the bundle protocol endpoint id which this CLA is exposing
	id bpv7.EndpointID
	// peerId is the bundle protocol endpoint id of the connected peer
	// This field may be nil until the initial handshake has happened
	peerId bpv7.EndpointID
	// The address in HOST:PORT format of the remote peer
	peerAddress string
	// The actual QUIC connection which transceives data
	connection quic.Connection

	dialer bool
	active bool

	// Whether the protocol handshake has been completed
	handshake *uint32
}

func NewListenerEndpoint(id bpv7.EndpointID, session quic.Connection) *Endpoint {
	return &Endpoint{
		id:          id,
		peerAddress: session.RemoteAddr().String(),
		connection:  session,
		dialer:      false,
		active:      false,
		handshake:   new(uint32),
	}
}

func NewDialerEndpoint(peerAddress string, id bpv7.EndpointID) *Endpoint {
	return &Endpoint{
		id:          id,
		peerAddress: peerAddress,
		dialer:      true,
		active:      false,
		handshake:   new(uint32),
	}
}

func (endpoint *Endpoint) String() string {
	return fmt.Sprintf("QUICLEndpoint{Peer ID: %v, Peer Address: %v, Dialer: %v}", endpoint.peerId, endpoint.peerAddress, endpoint.dialer)
}

/**
Methods for Convergable interface
*/

func (endpoint *Endpoint) Close() error {
	log.WithField("peer", endpoint.peerAddress).Debug("Someone called Close()")
	err := endpoint.connection.CloseWithError(internal.ApplicationShutdown, "Daemon shutting down")
	return err
}

/**
Methods for Convergence interface
*/

func (endpoint *Endpoint) Activate() error {
	log.WithFields(log.Fields{
		"cla":  endpoint.id,
		"peer": endpoint.peerAddress,
	}).Debug("Starting CLA")

	// if we are on the dialer-side we need to first initiate the quic-connection
	if endpoint.dialer {
		session, err := quic.DialAddr(context.Background(), endpoint.peerAddress, internal.GenerateSimpleDialerTLSConfig(), internal.GenerateQUICConfig())
		endpoint.connection = session
		if err != nil {
			return err
		}
		log.WithField("cla", endpoint.id).Debug("Dialer established QUIC connection")
	}

	var err error
	if endpoint.dialer {
		err = endpoint.handshakeDialer()
	} else {
		err = endpoint.handshakeListener()
	}

	if err != nil {
		var herr *internal.HandshakeError
		if errors.As(err, &herr) {
			log.WithFields(log.Fields{
				"cla":      endpoint,
				"error":    herr,
				"internal": herr.Unwrap(),
			}).Warn("Handshake failure")
			_ = endpoint.connection.CloseWithError(herr.Code, herr.Msg)
		} else {
			log.WithFields(log.Fields{
				"cla":   endpoint,
				"error": err,
			}).Error("Non handshake-related error during handshake")
			_ = endpoint.connection.CloseWithError(internal.LocalError, "Local error")
		}
		return err
	} else {
		go endpoint.handleConnection()
	}

	endpoint.active = true
	cla.GetManagerSingleton().NotifyConnect(endpoint.peerId)
	return nil
}

func (endpoint *Endpoint) Active() bool {
	return endpoint.active
}

func (endpoint *Endpoint) Address() string {
	return endpoint.peerAddress
}

/**
Methods for ConvergenceReceiver interface
*/

func (endpoint *Endpoint) GetEndpointID() bpv7.EndpointID {
	return endpoint.id
}

/**
Methods for ConvergenceSender interface
*/

func (endpoint *Endpoint) GetPeerEndpointID() bpv7.EndpointID {
	return endpoint.peerId
}

func (endpoint *Endpoint) Send(bndl bpv7.Bundle) error {
	log.WithFields(log.Fields{
		"peer":   endpoint.peerId,
		"bundle": bndl.ID(),
	}).Debug("Sending bundle")

	handshake := atomic.LoadUint32(endpoint.handshake)
	if handshake == 0 {
		return internal.NewInitialisationError("Handshake not yet completed")
	}

	stream, err := endpoint.connection.OpenStream()
	if err != nil {
		// TODO: understand possible error cases
		log.WithFields(log.Fields{
			"peer":   endpoint.peerId,
			"bundle": bndl.ID(),
			"error":  err,
		}).Debug("Error opening stream")

		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				cla.GetManagerSingleton().NotifyDisconnect(endpoint)
			}
		}

		return err
	}

	buff := new(bytes.Buffer)
	if err = cboring.Marshal(&bndl, buff); err != nil {
		log.WithFields(log.Fields{
			"peer":   endpoint.peerId,
			"bundle": bndl.ID(),
			"error":  err,
		}).Debug("Error marshaling data")

		stream.CancelWrite(internal.DataMarshalError)
		sErr := stream.Close()
		if sErr != nil {
			log.WithFields(log.Fields{
				"peer":   endpoint.peerId,
				"bundle": bndl.ID(),
				"error":  sErr,
			}).Debug("Error closing stream (marshaling error)")
		}
		return err
	}

	// TODO: Do we actually need the bufio-wrapper?
	writer := bufio.NewWriter(stream)
	if _, err = buff.WriteTo(writer); err != nil {
		log.WithFields(log.Fields{
			"peer":   endpoint.peerId,
			"bundle": bndl.ID(),
			"error":  err,
		}).Debug("Error writing to buffer")

		stream.CancelWrite(internal.StreamTransmissionError)
		sErr := stream.Close()
		if sErr != nil {
			log.WithFields(log.Fields{
				"peer":   endpoint.peerId,
				"bundle": bndl.ID(),
				"error":  sErr,
			}).Debug("Error closing stream (buffer-write error)")
		}
		return err
	}

	if err = writer.Flush(); err != nil {
		log.WithFields(log.Fields{
			"peer":   endpoint.peerId,
			"bundle": bndl.ID(),
			"error":  err,
		}).Debug("Error flushing buffer")

		stream.CancelWrite(internal.StreamTransmissionError)
		sErr := stream.Close()
		if sErr != nil {
			log.WithFields(log.Fields{
				"peer":   endpoint.peerId,
				"bundle": bndl.ID(),
				"error":  sErr,
			}).Debug("Error closing stream (flush error)")
		}

		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				cla.GetManagerSingleton().NotifyDisconnect(endpoint)
			}
		}
		return err
	}

	sErr := stream.Close()
	if sErr != nil {
		log.WithFields(log.Fields{
			"peer":   endpoint.peerId,
			"bundle": bndl.ID(),
			"error":  sErr,
		}).Debug("Error closing stream (send successful)")
	}

	log.WithFields(log.Fields{
		"peer":   endpoint.peerId,
		"bundle": bndl.ID(),
	}).Debug("Bundle sent")

	return nil
}

/*
Non-interface methods
*/

// handleConnection continuously listens on the connection and accepts incoming streams
// This method is meant to be run in its own goroutine.
// When a new stream is opened, i.e. when the peer wants to send us a bundle, we spawn a new goroutine
// to handle the incoming data.
func (endpoint *Endpoint) handleConnection() {
	log.WithFields(log.Fields{"endpoint": endpoint.GetEndpointID(), "peer": endpoint.GetPeerEndpointID()}).Debug("CLA Started")

	for {
		stream, err := endpoint.connection.AcceptStream(context.Background())
		log.WithField("CLA", endpoint).Debug("New incoming stream")
		if err != nil {
			var netErr net.Error
			var appErr *quic.ApplicationError

			switch {
			case errors.As(err, &netErr):
				if netErr.Timeout() {
					log.WithFields(log.Fields{
						"CLA":   endpoint,
						"error": netErr,
					}).Debug("Peer timed out.")

					cla.GetManagerSingleton().NotifyDisconnect(endpoint)

					return
				}

			case errors.As(err, &appErr):
				log.WithFields(log.Fields{
					"peer":       endpoint.peerId,
					"remote":     appErr.Remote,
					"error code": appErr.ErrorCode,
					"error msg":  appErr.ErrorMessage,
				}).Debug("Connection to peer closed")
				if appErr.Remote {
					cla.GetManagerSingleton().NotifyDisconnect(endpoint)
				}
				return

			default:
				log.WithFields(log.Fields{
					"CLA":   endpoint,
					"error": err,
				}).Error("Unexpected error while waiting for stream")
			}
		} else {
			go endpoint.handleStream(stream)
		}
	}
}

// handleStream hadles incoming bundles
// A single stream will always carry a single bundle, and will be closed once the bundle has been transmitted
func (endpoint *Endpoint) handleStream(stream quic.Stream) {
	log.WithField("cla", endpoint).Debug("Receiving bundle via quicl")

	// TODO: Do we actually need the bufio-wrapper?
	reader := bufio.NewReader(stream)

	bundle := new(bpv7.Bundle)
	if err := cboring.Unmarshal(bundle, reader); err != nil {
		log.WithFields(log.Fields{
			"cla":   endpoint,
			"error": err,
		}).Error("quicl failed to read bundle")

		stream.CancelRead(internal.StreamTransmissionError)
	} else {
		log.WithFields(log.Fields{
			"cla": endpoint,
		}).Debug("quicl received a bundle")

		cla.GetManagerSingleton().NotifyReceive(bundle)
	}
	log.WithField("cla", endpoint).Debug("Finished handling stream")
}

// handshakeListener performs the dialer-portion of the protocol handshake
// Since communication is initiated by the dialer, we listen on the connection for a new stream
// We then receive the dialer's EndpointID and finish by sending them ours
func (endpoint *Endpoint) handshakeListener() error {
	log.WithField("cla", endpoint.peerAddress).Debug("Performing listener handshake")

	// the dialer has half a second to initiate the handshake
	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()

	// wait for the dialer to open a stream
	stream, err := endpoint.connection.AcceptStream(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return internal.NewHandshakeError("dialer took too long to initiate handshake", internal.PeerError, err)
		} else {
			return internal.NewHandshakeError("unanticipated error happened", internal.UnknownError, err)
		}
	}

	// The listener first receives the dialer's ID
	if err = endpoint.receiveEndpointID(stream); err != nil {
		// TODO: close connection with error
		return err
	}

	// then send our own id
	if err = endpoint.sendEndpointID(stream); err != nil {
		return err
	}

	// lastly, close the stream
	if err = stream.Close(); err != nil {
		return internal.NewHandshakeError("error closing handshake stream", internal.ConnectionError, err)
	}

	atomic.StoreUint32(endpoint.handshake, 1)

	return nil
}

// handshakeDialer performs the dialer-portion of the protocol handshake
// We first open a new bidirectional data stream inside the QUIC connection
// We then send our own EndpointID over this stream, and finish by receiving the listener's id
func (endpoint *Endpoint) handshakeDialer() error {
	log.WithField("cla", endpoint.peerAddress).Debug("Performing dialer handshake")

	stream, err := endpoint.connection.OpenStream()
	if err != nil {
		return internal.NewHandshakeError("Error during stream initiation", internal.ConnectionError, err)
	}

	// start by sending own ID
	err = endpoint.sendEndpointID(stream)
	if err != nil {
		return err
	}

	// wait for the listener's ID
	err = endpoint.receiveEndpointID(stream)
	// TODO: if error, close stream

	atomic.StoreUint32(endpoint.handshake, 1)

	return err
}

// sendEndpointID sends this CLA's EndpointID (the one which is stored in the id-field) over a given QUIC stream.
// The EndpointID is first marshalled into a buffer using its builtin cboring marshaller.
// We then send the length of the buffer (using cboring ByteStringLen) followed by the ID itself.
func (endpoint *Endpoint) sendEndpointID(stream quic.Stream) error {
	log.WithField("cla", endpoint).Debug("Sending own endpoint id")

	buff := new(bytes.Buffer)
	if err := cboring.Marshal(&endpoint.id, buff); err != nil {
		return internal.NewHandshakeError("error marshaling endpoint-id", internal.LocalError, err)
	}

	// TODO: Do we actually need the bufio-wrapper?
	writer := bufio.NewWriter(stream)
	if err := cboring.WriteByteStringLen(uint64(buff.Len()), writer); err != nil {
		return internal.NewHandshakeError("error sending id length", internal.ConnectionError, err)
	}

	if _, err := buff.WriteTo(writer); err != nil {
		return internal.NewHandshakeError("error sending id", internal.ConnectionError, err)
	}

	if err := writer.Flush(); err != nil {
		return internal.NewHandshakeError("error flushing write-buffer", internal.ConnectionError, err)
	}

	return nil
}

// receiveEndpointID receives a remote CLA's EndpointID over a given QUIC stream
// The serialised form consists of the cbor representation of the EndpointID,
// wrapped in a cbor byte-string
func (endpoint *Endpoint) receiveEndpointID(stream quic.Stream) error {
	log.WithField("cla", endpoint).Debug("Receiving peer's endpoint id")
	reader := bufio.NewReader(stream)

	length, err := cboring.ReadByteStringLen(reader)
	if err != nil && !errors.Is(err, io.EOF) {
		return internal.NewHandshakeError("error reading id length", internal.ConnectionError, err)
	} else if length == 0 {
		return internal.NewHandshakeError("error reading id length", internal.ConnectionError, fmt.Errorf("length is 0"))
	}

	id := new(bpv7.EndpointID)
	if err = cboring.Unmarshal(id, reader); err != nil {
		// TODO: distinguish cbor and transmission errors
		return internal.NewHandshakeError("error reading id", internal.ConnectionError, err)
	}

	log.WithFields(log.Fields{
		"cla":     endpoint,
		"peer id": id,
	}).Debug("Received peer's endpoint id")

	endpoint.peerId = *id

	return nil
}
