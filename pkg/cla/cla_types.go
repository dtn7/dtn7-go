// SPDX-FileCopyrightText: 2020 Alvar Penning
// SPDX-FileCopyrightText: 2020, 2022 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cla

import (
	"errors"
)

// CLAType is one of the supported Convergence Layer Adaptors
type CLAType uint

const (
	// TCPCLv4 identifies the Delay-Tolerant Networking TCP Convergence Layer Protocol Version 4
	TCPCLv4 CLAType = 0

	// MTCP identifies the Minimal TCP Convergence-Layer Protocol, implemented in cla/mtcp.
	MTCP CLAType = 10

	QUICL CLAType = 20

	unknownClaTypeString string = "unknown CLA type"
)

// CheckValid checks if its value is known.
func (claType CLAType) CheckValid() (err error) {
	if claType.String() == unknownClaTypeString {
		err = errors.New(unknownClaTypeString)
	}
	return
}

func (claType CLAType) String() string {
	switch claType {
	case TCPCLv4:
		return "TCPCLv4"

	case MTCP:
		return "MTCP"

	case QUICL:
		return "QUICL"

	default:
		return unknownClaTypeString
	}
}
