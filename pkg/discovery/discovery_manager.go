// SPDX-FileCopyrightText: 2020, 2022, 2023 Markus Sommer
// SPDX-FileCopyrightText: 2020, 2021 Alvar Penning
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package discovery contains code for peer/neighbor discovery of other DTN nodes through UDP multicast packages.
package discovery

import (
	"fmt"
	"time"

	"github.com/schollz/peerdiscovery"
	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
	"github.com/dtn7/dtn7-go/pkg/cla/mtcp"
	"github.com/dtn7/dtn7-go/pkg/cla/quicl"
)

const (
	// address4 is the default multicast IPv4 address used for discovery.
	address4 = "224.23.23.23"

	// address6 is the default multicast IPv4 add6ess used for discovery.
	address6 = "ff02::23"

	// port is the default multicast UDP port used for discovery.
	port = 35039
)

// Manager publishes and receives Announcements.
type Manager struct {
	NodeId          bpv7.EndpointID
	receiveCallback func(*bpv7.Bundle)

	stopChan4 chan struct{}
	stopChan6 chan struct{}
}

var managerSingleton *Manager

func InitialiseManager(
	nodeId bpv7.EndpointID,
	announcements []Announcement, announcementInterval time.Duration,
	ipv4, ipv6 bool,
	receiveCallback func(*bpv7.Bundle)) error {
	if managerSingleton != nil {
		log.Fatalf("Attempting to access an uninitialised discovery manager. This must never happen!")
	}

	var manager = &Manager{
		NodeId:          nodeId,
		receiveCallback: receiveCallback,
	}
	if ipv4 {
		manager.stopChan4 = make(chan struct{})
	}
	if ipv6 {
		manager.stopChan6 = make(chan struct{})
	}

	log.WithFields(log.Fields{
		"interval":      announcementInterval,
		"IPv4":          ipv4,
		"IPv6":          ipv6,
		"announcements": announcements,
	}).Info("Starting discovery manager")

	msg, err := MarshalAnnouncements(announcements)
	if err != nil {
		return err
	}

	sets := []struct {
		active           bool
		multicastAddress string
		stopChan         chan struct{}
		ipVersion        peerdiscovery.IPVersion
		notify           func(discovered peerdiscovery.Discovered)
	}{
		{ipv4, address4, manager.stopChan4, peerdiscovery.IPv4, manager.notify},
		{ipv6, address6, manager.stopChan6, peerdiscovery.IPv6, manager.notify6},
	}

	for _, set := range sets {
		if !set.active {
			continue
		}

		set := peerdiscovery.Settings{
			Limit:            -1,
			Port:             fmt.Sprintf("%d", port),
			MulticastAddress: set.multicastAddress,
			Payload:          msg,
			Delay:            announcementInterval,
			TimeLimit:        -1,
			StopChan:         set.stopChan,
			AllowSelf:        true,
			IPVersion:        set.ipVersion,
			Notify:           set.notify,
		}

		discoverErrChan := make(chan error)
		go func() {
			_, discoverErr := peerdiscovery.Discover(set)
			discoverErrChan <- discoverErr
		}()

		select {
		case discoverErr := <-discoverErrChan:
			if discoverErr != nil {
				return discoverErr
			}

		case <-time.After(time.Second):
			break
		}
	}

	managerSingleton = manager
	return nil
}

// GetManagerSingleton returns the manager singleton-instance.
// Attempting to call this function before manager initialisation will cause the program to panic.
func GetManagerSingleton() *Manager {
	if managerSingleton == nil {
		log.Fatalf("Attempting to access an uninitialised discovery manager. This must never happen!")
	}
	return managerSingleton
}

func (manager *Manager) notify6(discovered peerdiscovery.Discovered) {
	discovered.Address = fmt.Sprintf("[%s]", discovered.Address)

	manager.notify(discovered)
}

func (manager *Manager) notify(discovered peerdiscovery.Discovered) {
	announcements, err := UnmarshalAnnouncements(discovered.Payload)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"discovery": manager,
			"peer":      discovered.Address,
		}).Warn("Peer discovery failed to parse incoming package")

		return
	}

	for _, announcement := range announcements {
		go manager.handleDiscovery(announcement, discovered.Address)
	}
}

func (manager *Manager) handleDiscovery(announcement Announcement, addr string) {
	if manager.NodeId.SameNode(announcement.Endpoint) {
		return
	}

	/*
		log.WithFields(log.Fields{
			"peer":    addr,
			"message": announcement,
		}).Debug("Peer discovery received a message")
	*/

	var conv cla.Convergence
	switch announcement.Type {
	case cla.MTCP:
		conv = mtcp.NewMTCPClient(fmt.Sprintf("%s:%d", addr, announcement.Port), announcement.Endpoint)
	case cla.QUICL:
		conv = quicl.NewDialerEndpoint(fmt.Sprintf("%s:%d", addr, announcement.Port), manager.NodeId, manager.receiveCallback)
	default:
		log.WithField("cType", announcement.Type).Error("Invalid cType")
		return
	}
	cla.GetManagerSingleton().Register(conv)
}

// Close this Manager.
func (manager *Manager) Close() {
	for _, c := range []chan struct{}{manager.stopChan4, manager.stopChan6} {
		if c != nil {
			c <- struct{}{}
		}
	}
}

func (manager *Manager) String() string {
	return fmt.Sprintf("Manager(%v)", manager.NodeId)
}
