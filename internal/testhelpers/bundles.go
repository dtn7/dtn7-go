// SPDX-License-Identifier: GPL-3.0-or-later

// Package testhelpers provides test data generators shared between packages.
// It must only be imported by tests.
package testhelpers

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/dtn7/dtn7-go/pkg/bpv7"
)

// RandomBundle generates a bundle with random endpoints and payload.
var RandomBundle = rapid.Custom(func(t *rapid.T) *bpv7.Bundle {
	bndl, err := bpv7.Builder().
		CRC(bpv7.CRC32).
		Source(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(t, "sourceID")).
		Destination(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(t, "destinationID")).
		CreationTimestampNow().
		Lifetime("10m").
		HopCountBlock(64).
		BundleAgeBlock(0).
		PayloadBlock([]byte(rapid.String().Draw(t, "payload"))).
		Build()
	if err != nil {
		t.Fatalf("Error during bundle creation %s", err)
	}
	return bndl
})

// SampleBundle returns a fixed bundle for tests that do not need randomness.
func SampleBundle(t testing.TB) *bpv7.Bundle {
	t.Helper()

	bndl, err := bpv7.Builder().
		CRC(bpv7.CRC32).
		Source("dtn://myself/").
		Destination("dtn://dest/").
		CreationTimestampEpoch().
		Lifetime("10m").
		HopCountBlock(64).
		BundleAgeBlock(0).
		PayloadBlock([]byte("hello world!")).
		Build()
	if err != nil {
		t.Fatalf("Error during bundle creation %s", err)
	}
	return bndl
}
