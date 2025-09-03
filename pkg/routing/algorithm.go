// SPDX-FileCopyrightText: 2023, 2025 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package routing provides and interface & implementations for routing algorithms.
//
// Since there should only be a single Algorithm active at any time, this package employs the singleton pattern.
// Use `InitialiseAlgorithm` and `GetAlgorithmSingleton.`
package routing

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
	"github.com/dtn7/dtn7-go/pkg/cla"
	"github.com/dtn7/dtn7-go/pkg/store"
	"github.com/dtn7/dtn7-go/pkg/util"
)

type AlgorithmEnum uint32

const (
	Epidemic AlgorithmEnum = iota
)

func AlgorithmEnumFromString(name string) (AlgorithmEnum, error) {
	switch name = strings.ToLower(name); name {
	case "epidemic":
		return Epidemic, nil
	default:
		return 0, fmt.Errorf("%s is not a valid algorithm name", name)
	}
}

// Algorithm is an interface to specify routing algorithms for delay-tolerant networks.
type Algorithm interface {
	// NotifyNewBundle notifies this Algorithm about new bundles. They
	// might be generated at this node or received from a peer. Whether an
	// algorithm acts on this information or ignores it, is an implementation matter.
	NotifyNewBundle(descriptor *store.BundleDescriptor)

	// SelectPeersForForwarding returns an array of ConvergenceSender for a requested bundle.
	// The CLA selection is based on the algorithm's design.
	SelectPeersForForwarding(descriptor *store.BundleDescriptor) (peers []cla.ConvergenceSender)

	// NotifyPeerAppeared notifies the Algorithm about a new peer.
	NotifyPeerAppeared(peer bpv7.EndpointID)

	// NotifyPeerDisappeared notifies the Algorithm about the
	// disappearance of a peer.
	NotifyPeerDisappeared(peer bpv7.EndpointID)
}

var algorithmSingleton Algorithm

type NoSuchAlgorithmError AlgorithmEnum

func (err *NoSuchAlgorithmError) Error() string {
	return fmt.Sprintf("%d was already initialised", *err)
}

func NewNoSuchAlgorithmError(algorithm AlgorithmEnum) *NoSuchAlgorithmError {
	err := NoSuchAlgorithmError(algorithm)
	return &err
}

func InitialiseAlgorithm(algorithm AlgorithmEnum) error {
	if algorithmSingleton != nil {
		return util.NewAlreadyInitialisedError("Routing Algorithm")
	}

	if algorithm == Epidemic {
		algorithmSingleton = NewEpidemicRouting()
		return nil
	}

	return NewNoSuchAlgorithmError(algorithm)
}

// GetAlgorithmSingleton returns the routing algorithm singleton-instance.
// Attempting to call this function before algorithm initialisation will cause the program to panic.
func GetAlgorithmSingleton() Algorithm {
	if algorithmSingleton == nil {
		log.Fatalf("Attempting to access an uninitialised manager. This must never happen!")
	}
	return algorithmSingleton
}

// filterCLAs filters the nodes which already received a Bundle.
// It returns a list of unused ConvergenceSenders.
func filterCLAs(bundleDescriptor *store.BundleDescriptor, clas []cla.ConvergenceSender) (filtered []cla.ConvergenceSender) {
	filtered = make([]cla.ConvergenceSender, 0, len(clas))

	sentEids := bundleDescriptor.GetAlreadySent()

	for _, cs := range clas {
		skip := false

		for _, eid := range sentEids {
			if cs.GetPeerEndpointID() == eid {
				skip = true
				break
			}
		}

		if !skip {
			filtered = append(filtered, cs)
		}
	}

	return
}
