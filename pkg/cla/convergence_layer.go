// SPDX-FileCopyrightText: 2019, 2020 Alvar Penning
// SPDX-FileCopyrightText: 2023 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package cla defines two interfaces for Convergence Layer Adapters.
//
// The ConvergenceReceiver specifies a type which receives bundles and forwards
// those to an exposed channel.
//
// The ConvergenceSender specifies a type which sends bundles to a remote
// endpoint.
//
// An implemented convergence layer can be a ConvergenceReceiver,
// ConvergenceSender or even both. This depends on the convergence layer's
// specification and is an implementation matter.
//
// Furthermore, the ConvergenceProvider provides the ability to create new
// instances of Convergence objects.
//
// Those types are generalized by the Convergable interface.
//
// A centralized instance for CLA management offers the Manager, designed to
// work seamlessly with the types above.
package cla

import (
	"io"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
)

// Convergence is an interface to describe all kinds of Convergence Layer
// Adapters. There should not be a direct implementation of this interface. One
// must implement ConvergenceReceiver and/or ConvergenceSender, which are both
// extending this interface.
// A type can be both a ConvergenceReceiver and ConvergenceSender.
type Convergence interface {
	io.Closer

	// Start this Convergence{Receiver,Sender}
	Start() error

	// Address should return a unique address string to both identify this
	// Convergence{Receiver,Sender} and ensure it will not be opened twice.

	// TODO: The way this works right now does have some problems.
	// If you're using host:port to identify a CLA you might end up with multiple connections
	// between two nodes. If both are sending neighbour-discovery announcements, they will include
	// their listener-port which will be different from the client-port of an existing connection.
	// For mtcp, you actually need that, since the CLA pretends to be unidirectional, but if you have
	// full duplex communication it's unnecessary to have to CLAs for each node-pair.
	// But fixing this would probably require some major changes to the manager.
	Address() string

	// IsPermanent returns true, if this CLA should not be removed after failures.
	IsPermanent() bool

	// TODO: String method for address-logging
}

// ConvergenceReceiver is an interface for types which are able to receive
// bundles from other nodes.
type ConvergenceReceiver interface {
	Convergence

	// GetEndpointID returns the endpoint ID assigned to this CLA.
	GetEndpointID() bpv7.EndpointID
}

// ConvergenceSender is an interface for types which are able to transmit
// bundles to another node.
type ConvergenceSender interface {
	Convergence

	// Send a bundle to this ConvergenceSender's endpoint. This method should be thread safe.
	Send(bpv7.Bundle) error

	// GetPeerEndpointID returns the endpoint ID assigned to this CLA's peer,
	// if it's known. Otherwise, the zero endpoint will be returned.
	GetPeerEndpointID() bpv7.EndpointID
}
