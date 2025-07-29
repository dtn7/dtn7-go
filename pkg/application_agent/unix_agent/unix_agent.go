// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package unix_agent

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/dtn7/dtn7-go/pkg/application_agent"
	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/store"
)

// UNIXAgent allows for communication with dtnd via a UNIX domain socket
type UNIXAgent struct {
	listenAddress *net.UnixAddr
	listener      *net.UnixListener
	mailboxes     map[bpv7.EndpointID]bpv7.Bundle
	mailboxMutex  sync.Mutex
	stopChan      chan interface{}
}

func NewUNIXAgent(listenAddress string) (*UNIXAgent, error) {
	unixAddr, err := net.ResolveUnixAddr("unix", listenAddress)
	if err != nil {
		return nil, err
	}

	agent := UNIXAgent{
		listenAddress: unixAddr,
		mailboxes:     make(map[bpv7.EndpointID]bpv7.Bundle),
		stopChan:      make(chan interface{}),
	}
	return &agent, nil
}

func (agent *UNIXAgent) Shutdown() {
	log.WithField("listenAddress", agent.listenAddress).Info("Shutting agent down")
	close(agent.stopChan)
	f, _ := agent.listener.File()
	_ = f.Close()
	_ = agent.listener.Close()
}

func (agent *UNIXAgent) Endpoints() []bpv7.EndpointID {
	return []bpv7.EndpointID{}
}

func (agent *UNIXAgent) Deliver(bundleDescriptor *store.BundleDescriptor) error {
	return nil
}

func (agent *UNIXAgent) Start() error {
	log.WithFields(log.Fields{
		"address": agent.listenAddress,
	}).Info("Starting UNIXAgent")

	listener, err := net.ListenUnix("unix", agent.listenAddress)
	if err != nil {
		return err
	}
	agent.listener = listener

	go agent.listen()

	return nil
}

func (agent *UNIXAgent) listen() {
	defer func() {
		log.WithField("listenAddress", agent.listenAddress).Info("Cleaning up socket")
	}()

	for {
		select {
		case <-agent.stopChan:
			return

		default:
			if err := agent.listener.SetDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
				log.WithFields(log.Fields{
					"listener": agent.listener,
					"error":    err,
				}).Error("UNIXAgent failed to set deadline on UNIX socket")

				agent.Shutdown()
			} else if conn, err := agent.listener.Accept(); err == nil {
				go agent.handleConnection(conn)
			}
		}
	}
}

func (agent *UNIXAgent) handleConnection(conn net.Conn) {
	defer conn.Close()

	connReader := bufio.NewReader(conn)
	connWriter := bufio.NewWriter(conn)

	msgLengthBytes := make([]byte, 8)
	log.Debug("Receiving message length")
	_, err := io.ReadFull(connReader, msgLengthBytes)
	if err != nil {
		log.WithField("error", err).Error("Failed reading 8-byte message length")
		return
	}

	msgLength := binary.BigEndian.Uint64(msgLengthBytes)
	log.WithField("msgLength", msgLength).Debug("Received msgLength")

	log.Debug("Receiving message")
	msgBytes := make([]byte, msgLength)
	_, err = io.ReadFull(connReader, msgBytes)
	if err != nil {
		log.WithField("error", err).Error("Failed reading message")
		return
	}

	log.Debug("Unmarshaling message")
	message := Message{}
	err = msgpack.Unmarshal(msgBytes, &message)
	if err != nil {
		log.WithField("error", err).Error("Failed unmarshaling message")
		return
	}
	log.WithField("type", message.Type).Debug("Received message")

	var replyBytes []byte
	switch message.Type {
	case 1:
		createMessage := BundleCreate{}
		err = msgpack.Unmarshal(msgBytes, &createMessage)
		if err != nil {
			log.WithField("error", err).Error("Failed unmarshaling Bundle create message")
			return
		}
		log.WithField("message", createMessage).Debug("Typed message")
		replyBytes, err = agent.handleBundleCreate(&createMessage)
		if err != nil {
			log.WithField("error", err).Error("Error handling Bundle create message")
			return
		}
	default:
		log.Debug("Not doing anything with this message")
		return
	}

	replyLength := uint64(len(replyBytes))
	replyLengthBytes := make([]byte, 8)
	_, err = binary.Encode(replyLengthBytes, binary.BigEndian, replyLength)
	if err != nil {
		log.WithField("error", err).Error("Error encoding reply length")
		return
	}

	_, err = connWriter.Write(replyLengthBytes)
	if err != nil {
		log.WithField("error", err).Error("Error sending reply length")
		return
	}
	err = connWriter.Flush()
	if err != nil {
		log.WithField("error", err).Error("Error sending reply length")
		return
	}

	_, err = connWriter.Write(replyBytes)
	if err != nil {
		log.WithField("error", err).Error("Error sending reply")
		return
	}
	err = connWriter.Flush()
	if err != nil {
		log.WithField("error", err).Error("Error sending reply")
		return
	}
}

func (agent *UNIXAgent) handleBundleCreate(message *BundleCreate) ([]byte, error) {
	log.Debug("Handling bundle create")

	reply := BundleCreateResponse{
		Message: Message{Type: MsgTypeBundleCreateResponse},
		Success: true,
		Error:   "",
	}

	failed := false
	bldr := bpv7.Builder()
	bldr.Lifetime("60m")
	bldr.CreationTimestampNow()
	bldr.PayloadBlock(message.Payload)

	log.WithField("eid", message.Source).Debug("Parsing source id")
	sourceID, err := bpv7.NewEndpointID(message.Source)
	if err != nil {
		failed = true
		reply.Success = false
		reply.Error = err.Error()
	} else {
		bldr.Source(sourceID)
	}

	if !failed {
		log.WithField("eid", message.Destination).Debug("Parsing destination id")
		destinationID, err := bpv7.NewEndpointID(message.Destination)
		if err != nil {
			failed = true
			reply.Success = false
			reply.Error = err.Error()
		} else {
			bldr.Destination(destinationID)
		}
	}

	log.Debug("Building bundle")
	bundle, err := bldr.Build()
	if err != nil {
		log.WithField("error", err).Debug("Error building bundle")
		failed = true
		reply.Success = false
		reply.Error = err.Error()
	}

	if !failed {
		log.WithField("bundle", bundle).Debug("Handing bundle over to manager")
		application_agent.GetManagerSingleton().Send(bundle)
	}

	log.Debug("Marshaling response")
	replyBytes, err := msgpack.Marshal(&reply)
	if err != nil {
		log.WithField("error", err).Error("Response marshaling error")
		return nil, err
	}

	return replyBytes, nil
}
