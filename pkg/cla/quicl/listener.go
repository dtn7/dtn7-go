// SPDX-FileCopyrightText: 2022 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package quicl

import (
	"context"
	"errors"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
	"github.com/dtn7/dtn7-ng/pkg/cla/quicl/internal"
	"github.com/quic-go/quic-go"
	log "github.com/sirupsen/logrus"
)

type Listener struct {
	listenAddress string
	endpointID    bpv7.EndpointID
	quicListener  *quic.Listener
	running       bool

	registerCallback func(id bpv7.EndpointID)
}

func NewQUICListener(listenAddress string, endpointID bpv7.EndpointID) *Listener {
	return &Listener{
		listenAddress: listenAddress,
		endpointID:    endpointID,
		quicListener:  nil,
		running:       false,
	}
}

func (listener *Listener) Close() error {
	log.WithField("address", listener.listenAddress).Info("Shutting ourselves down")
	return listener.quicListener.Close()
}

func (listener *Listener) Start() error {
	log.WithField("address", listener.listenAddress).Info("Starting QUICL-listener")
	lst, err := quic.ListenAddr(listener.listenAddress, internal.GenerateSimpleListenerTLSConfig(), internal.GenerateQUICConfig())
	if err != nil {
		log.WithError(err).Error("Error creating QUICL listener")
		return err
	}

	listener.quicListener = lst
	go listener.handle()

	listener.running = true

	return nil
}

func (listener *Listener) Running() bool {
	return listener.running
}

func (listener *Listener) Address() string {
	return listener.listenAddress
}

/*
Non-interface methods
*/

func (listener *Listener) handle() {
	log.WithField("address", listener.listenAddress).Info("Listening for QUICL connections")

	for {
		session, err := listener.quicListener.Accept(context.Background())
		if err != nil {
			if !(errors.Is(err, context.DeadlineExceeded)) {
				if err.Error() == "quic: Server closed" {
					log.WithField("address", listener.listenAddress).Info("Shutting this place down")
					return
				}

				log.WithFields(log.Fields{
					"address": listener.listenAddress,
					"error":   err,
				}).Error("Unknown error accepting QUIC connection")
			}
		} else {
			log.WithFields(log.Fields{
				"address": listener.listenAddress,
				"peer":    session.RemoteAddr(),
			}).Info("QUICL listener accepted new connection")
			endpoint := NewListenerEndpoint(listener.endpointID, session)
			cla.GetManagerSingleton().Register(endpoint)
		}
	}
}
