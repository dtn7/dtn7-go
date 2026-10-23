// SPDX-License-Identifier: GPL-3.0-or-later

package testhelpers

import (
	"net"
	"strconv"

	"pgregory.net/rapid"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

// RandomEndpointID generates a random DTN endpoint ID that is not dtn:none.
var RandomEndpointID = rapid.Custom(func(t *rapid.T) bpv7.EndpointID {
	return bpv7.MustNewEndpointID(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(t, "eid"))
})

// RandomPort returns a currently free port for the given network ("tcp" or "udp").
func RandomPort(t *rapid.T, network string) int {
	l, err := net.Listen(network, "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
