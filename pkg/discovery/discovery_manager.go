// SPDX-FileCopyrightText: 2020, 2022, 2023 Markus Sommer
// SPDX-FileCopyrightText: 2020, 2021 Alvar Penning
//
// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/schollz/peerdiscovery"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"github.com/dtn7/dtn7-ng/pkg/cla"
)

// DiscoveryManager publishes and receives Announcements.
type DiscoveryManager struct {
	NodeId       bpv7.EndpointID
	RegisterFunc func(cType cla.CLAType, address string, peerID bpv7.EndpointID) `json:"-"`

	stopChan4 chan struct{}
	stopChan6 chan struct{}
}

var ManagerSingleton *DiscoveryManager

func InitialiseManager(
	nodeId bpv7.EndpointID, registerFunc func(cType cla.CLAType, address string, peerID bpv7.EndpointID),
	announcements []Announcement, announcementInterval time.Duration,
	ipv4, ipv6 bool) {

	var manager = &DiscoveryManager{
		NodeId:       nodeId,
		RegisterFunc: registerFunc,
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
	}).Info("Starting Manager")

	msg, err := MarshalAnnouncements(announcements)
	if err != nil {
		log.WithField("error", err).Fatal("Error initialising DiscoveryManager")
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
				log.WithField("error", err).Error("Discovery Error")
			}

		case <-time.After(time.Second):
			break
		}
	}

	ManagerSingleton = manager
}

func (manager *DiscoveryManager) notify6(discovered peerdiscovery.Discovered) {
	discovered.Address = fmt.Sprintf("[%s]", discovered.Address)

	manager.notify(discovered)
}

func (manager *DiscoveryManager) notify(discovered peerdiscovery.Discovered) {
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

func (manager *DiscoveryManager) handleDiscovery(announcement Announcement, addr string) {
	if manager.NodeId.SameNode(announcement.Endpoint) {
		return
	}

	log.WithFields(log.Fields{
		"discovery": manager,
		"peer":      addr,
		"message":   announcement,
	}).Debug("Peer discovery received a message")

	manager.RegisterFunc(announcement.Type, fmt.Sprintf("%s:%d", addr, announcement.Port), announcement.Endpoint)
}

// Close this Manager.
func (manager *DiscoveryManager) Close() {
	for _, c := range []chan struct{}{manager.stopChan4, manager.stopChan6} {
		if c != nil {
			c <- struct{}{}
		}
	}
}

func (manager *DiscoveryManager) String() string {
	return fmt.Sprintf("Manager(%v)", manager.NodeId)
}
