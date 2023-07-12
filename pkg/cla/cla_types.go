// SPDX-FileCopyrightText: 2020 Alvar Penning
// SPDX-FileCopyrightText: 2020, 2022, 2023 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cla

import (
	"errors"
	"fmt"
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

func TypeFromString(claType string) (CLAType, error) {
	switch claType {
	case "TCPCLv4":
		return TCPCLv4, nil
	case "MTCP":
		return MTCP, nil
	case "QUICL":
		return QUICL, nil
	default:
		return 0, fmt.Errorf("invalid CLA Type: %v", claType)
	}
}

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

type ListenerConfig struct {
	Type    CLAType
	Address string
}
