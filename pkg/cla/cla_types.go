// SPDX-FileCopyrightText: 2020 Alvar Penning
// SPDX-FileCopyrightText: 2020, 2022, 2023 Markus Sommer
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cla

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
)

// CLAType is one of the supported Convergence Layer Adaptors
type CLAType uint

const (
	// Delay-Tolerant Networking TCP Convergence Layer Protocol Version 4
	// As specified in RFC9174: https://datatracker.ietf.org/doc/html/rfc9174
	TCPCLv4 CLAType = 0

	// There originally existed an RFC draft for a "Minimal TCP Convergence Layer",
	// but this has expired and not seen any activity in a long time... so not sure where we stand there
	// https://datatracker.ietf.org/doc/html/draft-ietf-dtn-mtcpcl-01
	MTCP CLAType = 10

	QUICL CLAType = 20

	// Dummy CLA used for testing
	Dummy CLAType = 8080

	unknownClaTypeString string = "unknown CLA type"
)

func TypeFromString(claType string) (CLAType, error) {
	claType = strings.ToLower(claType)
	switch claType {
	case "tcpclv4":
		return TCPCLv4, nil
	case "mtcp":
		return MTCP, nil
	case "quicl":
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

type UnsupportedCLATypeError CLAType

func NewUnsupportedCLATypeError(claType CLAType) *UnsupportedCLATypeError {
	err := UnsupportedCLATypeError(claType)
	return &err
}

func (err *UnsupportedCLATypeError) Error() string {
	return fmt.Sprintf("%s is not a supported CLA type", CLAType(*err))
}

type ListenerConfig struct {
	Type       CLAType
	Address    string
	EndpointId bpv7.EndpointID
}
