// SPDX-FileCopyrightText: 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package unix_agent

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
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
	mailboxes     *application_agent.MailboxBank
	stopChan      chan interface{}
}

func NewUNIXAgent(listenAddress string) (*UNIXAgent, error) {
	unixAddr, err := net.ResolveUnixAddr("unix", listenAddress)
	if err != nil {
		return nil, err
	}

	agent := UNIXAgent{
		listenAddress: unixAddr,
		mailboxes:     application_agent.NewMailboxBank(),
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
	return agent.mailboxes.RegisteredIDs()
}

func (agent *UNIXAgent) Deliver(bundleDescriptor *store.BundleDescriptor) error {
	return agent.mailboxes.Deliver(bundleDescriptor)
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

	msgLenBytes := make([]byte, 8)
	log.Debug("Receiving message length")
	_, err := io.ReadFull(connReader, msgLenBytes)
	if err != nil {
		log.WithField("error", err).Error("Failed reading 8-byte message length")
		return
	}

	msgLen := binary.BigEndian.Uint64(msgLenBytes)
	log.WithField("msgLength", msgLen).Debug("Received msgLength")

	log.Debug("Receiving message")
	msgBytes := make([]byte, msgLen)
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
	case MsgTypeRegisterEID, MsgTypeUnregisterEID:
		typedMessage := RegisterUnregisterMessage{}
		err = msgpack.Unmarshal(msgBytes, &typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Failed unmarshaling Bundle create message")
			return
		}

		replyBytes, err = agent.handleRegisterUnregister(&typedMessage, typedMessage.Message.Type == MsgTypeRegisterEID)

		if err != nil {
			log.WithField("error", err).Error("Error handling Bundle create message")
			return
		}
	case MsgTypeBundleCreate:
		typedMessage := BundleCreateMessage{}
		err = msgpack.Unmarshal(msgBytes, &typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Failed unmarshaling Bundle create message")
			return
		}
		replyBytes, err = agent.handleBundleCreate(&typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Error handling Bundle create message")
			return
		}
	case MsgTypeList:
		typedMessage := MailboxListMessage{}
		err = msgpack.Unmarshal(msgBytes, &typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Failed unmarshaling list message")
			return
		}
		replyBytes, err = agent.handleMailboxList(&typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Error handling list message")
			return
		}
	case MsgTypeGetBundle:
		typedMessage := GetBundleMessage{}
		err = msgpack.Unmarshal(msgBytes, &typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Failed unmarshaling get message")
			return
		}
		replyBytes, err = agent.handleGetBundle(&typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Error handling get message")
			return
		}
	case MsgTypeGetAllBundles:
		typedMessage := GetAllBundlesMessage{}
		err = msgpack.Unmarshal(msgBytes, &typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Failed unmarshaling getall message")
			return
		}
		replyBytes, err = agent.handleGetAllBundles(&typedMessage)
		if err != nil {
			log.WithField("error", err).Error("Error handling getall message")
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
	_, err = connWriter.Write(replyBytes)
	if err != nil {
		log.WithField("error", err).Error("Error sending reply")
		return
	}
	err = connWriter.Flush()
	if err != nil {
		log.WithField("error", err).Error("Error flushing send buffer")
		return
	}
}

func (agent *UNIXAgent) handleRegisterUnregister(message *RegisterUnregisterMessage, register bool) ([]byte, error) {
	if register {
		log.WithField("eid", message.EndpointID).Info("Received registration request")
	} else {
		log.WithField("eid", message.EndpointID).Info("Received deregistration request")
	}

	response := GeneralResponse{
		Message: Message{Type: MsgTypeGeneralResponse},
		Success: true,
		Error:   "",
	}
	failure := false

	eid, err := bpv7.NewEndpointID(message.EndpointID)
	if err != nil {
		failure = true
		response.Success = false
		response.Error = err.Error()
		log.WithFields(log.Fields{
			"eid":   message.EndpointID,
			"error": err,
		}).Debug("Error parsing EndpointID")
	}

	if !failure {
		if register {
			err = agent.mailboxes.Register(eid)
		} else {
			err = agent.mailboxes.Unregister(eid)
		}
	}
	if err != nil {
		failure = true
		response.Success = false
		response.Error = err.Error()
		log.WithFields(log.Fields{
			"eid":   message.EndpointID,
			"error": err,
		}).Debug("Error performing (un)registration")
	}

	log.Debug("Marshaling response")
	responseBytes, err := msgpack.Marshal(&response)
	if err != nil {
		log.WithField("error", err).Error("Response marshaling error")
		return nil, err
	}

	return responseBytes, nil
}

func (agent *UNIXAgent) handleBundleCreate(message *BundleCreateMessage) ([]byte, error) {
	log.Debug("Handling bundle create")

	response := GeneralResponse{
		Message: Message{Type: MsgTypeGeneralResponse},
		Success: true,
		Error:   "",
	}

	failed := false
	bundle, err := bpv7.BuildFromMap(message.Args)
	if err != nil {
		log.WithField("error", err).Debug("Error building bundle")
		failed = true
		response.Success = false
		response.Error = err.Error()
	}

	if !failed {
		log.WithField("bundle", bundle).Debug("Handing bundle over to manager")
		application_agent.GetManagerSingleton().Send(bundle)
	}

	log.Debug("Marshaling response")
	responseBytes, err := msgpack.Marshal(&response)
	if err != nil {
		log.WithField("error", err).Error("Response marshaling error")
		return nil, err
	}

	return responseBytes, nil
}

func (agent *UNIXAgent) handleMailboxList(message *MailboxListMessage) ([]byte, error) {
	log.Debug("Handling mailbox list")

	response := MailboxListResponse{
		GeneralResponse: GeneralResponse{
			Message: Message{Type: MsgTypeListResponse},
			Success: true,
			Error:   "",
		},
		Bundles: make([]string, 0),
	}

	failure := false
	eid, err := bpv7.NewEndpointID(message.Mailbox)
	if err != nil {
		failure = true
		response.Success = false
		response.Error = err.Error()
		log.WithFields(log.Fields{
			"eid":   message.Mailbox,
			"error": err,
		}).Debug("Error parsing EndpointID")
	}

	var mailbox *application_agent.Mailbox
	if !failure {
		mailbox, err = agent.mailboxes.GetMailbox(eid)
		if err != nil {
			failure = true
			response.Success = false
			response.Error = err.Error()
			log.WithFields(log.Fields{
				"eid":   message.Mailbox,
				"error": err,
			}).Debug("Error getting mailbox")
		}
	}

	if !failure {
		var bundles []bpv7.BundleID
		if message.New {
			bundles = mailbox.ListNew()
		} else {
			bundles = mailbox.List()
		}
		bundlesStr := make([]string, len(bundles))
		for i := range bundles {
			bundlesStr[i] = bundles[i].String()
		}
		response.Bundles = bundlesStr
		log.WithFields(log.Fields{
			"eid":     message.Mailbox,
			"bundles": bundlesStr,
		}).Debug("Got list of bundles")
	}

	log.Debug("Marshaling response")
	responseBytes, err := msgpack.Marshal(&response)
	if err != nil {
		log.WithField("error", err).Error("Response marshaling error")
		return nil, err
	}

	return responseBytes, nil
}

func (agent *UNIXAgent) handleGetBundle(message *GetBundleMessage) ([]byte, error) {
	log.Debug("Handling get bundle")
	response := GetBundleResponse{
		GeneralResponse: GeneralResponse{
			Message: Message{Type: MsgTypeGetBundleResponse},
			Success: true,
			Error:   "",
		},
		BundleContent: BundleContent{},
	}

	failure := false
	mid, err := bpv7.NewEndpointID(message.Mailbox)
	if err != nil {
		failure = true
		response.Success = false
		response.Error = err.Error()
		log.WithFields(log.Fields{
			"bid":   message.Mailbox,
			"error": err,
		}).Debug("Error parsing EndpointID")
	}

	var mailbox *application_agent.Mailbox
	if !failure {
		mailbox, err = agent.mailboxes.GetMailbox(mid)
		if err != nil {
			failure = true
			response.Success = false
			response.Error = err.Error()
			log.WithFields(log.Fields{
				"eid":   message.Mailbox,
				"error": err,
			}).Debug("Error getting mailbox")
		}
	}

	var bid bpv7.BundleID
	if !failure {
		bid, err = bpv7.NewBundleID(message.BundleID)
		if err != nil {
			failure = true
			response.Success = false
			response.Error = err.Error()
			log.WithFields(log.Fields{
				"eid":   message.Mailbox,
				"error": err,
			}).Debug("Error parsing BundleID")
		}
	}

	if !failure {
		bundle, err := mailbox.Get(bid, message.Remove)
		if err != nil {
			failure = true
			response.Success = false
			response.Error = err.Error()
			log.WithFields(log.Fields{
				"eid":   message.Mailbox,
				"error": err,
			}).Debug("Error retrieving bundle")
		} else {
			response.BundleID = bundle.ID().String()
			response.SourceID = bundle.PrimaryBlock.SourceNode.String()
			response.DestinationID = bundle.PrimaryBlock.Destination.String()
			response.Payload = *bundle.PayloadBlock.Value.(*bpv7.PayloadBlock)
		}
	}

	log.Debug("Marshaling response")
	responseBytes, err := msgpack.Marshal(&response)
	if err != nil {
		log.WithField("error", err).Error("Response marshaling error")
		return nil, err
	}

	return responseBytes, nil
}

func (agent *UNIXAgent) handleGetAllBundles(message *GetAllBundlesMessage) ([]byte, error) {
	log.Debug("Handling get all bundles")
	response := GetAllBundlesResponse{
		GeneralResponse: GeneralResponse{
			Message: Message{Type: MsgTypeGetAllBundlesResponse},
			Success: true,
			Error:   "",
		},
		Bundles: make([]BundleContent, 0),
	}

	failure := false
	mid, err := bpv7.NewEndpointID(message.Mailbox)
	if err != nil {
		failure = true
		response.Success = false
		response.Error = err.Error()
		log.WithFields(log.Fields{
			"bid":   message.Mailbox,
			"error": err,
		}).Debug("Error parsing EndpointID")
	}

	var mailbox *application_agent.Mailbox
	if !failure {
		mailbox, err = agent.mailboxes.GetMailbox(mid)
		if err != nil {
			failure = true
			response.Success = false
			response.Error = err.Error()
			log.WithFields(log.Fields{
				"eid":   message.Mailbox,
				"error": err,
			}).Debug("Error getting mailbox")
		}
	}

	if !failure {
		var bundles []*bpv7.Bundle
		if message.New {
			bundles, err = mailbox.GetNew(message.Remove)
		} else {
			bundles, err = mailbox.GetAll(message.Remove)
		}
		if err != nil {
			failure = true
			response.Success = false
			response.Error = err.Error()
			log.WithFields(log.Fields{
				"eid":   message.Mailbox,
				"error": err,
			}).Debug("Error retrieving bundle")
		} else {
			bundleContents := make([]BundleContent, len(bundles))
			for i, bundle := range bundles {
				bundleContent := BundleContent{
					BundleID:      bundle.ID().String(),
					SourceID:      bundle.PrimaryBlock.SourceNode.String(),
					DestinationID: bundle.PrimaryBlock.Destination.String(),
					Payload:       *bundle.PayloadBlock.Value.(*bpv7.PayloadBlock),
				}
				bundleContents[i] = bundleContent
			}
			response.Bundles = bundleContents
		}
	}

	log.Debug("Marshaling response")
	responseBytes, err := msgpack.Marshal(&response)
	if err != nil {
		log.WithField("error", err).Error("Response marshaling error")
		return nil, err
	}

	return responseBytes, nil
}
