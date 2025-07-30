package bpv7

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

func GenerateSampleBundle(t *testing.T) *Bundle {
	bndl, err := Builder().
		CRC(CRC32).
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

func GenerateRandomizedBundle(t *rapid.T, i uint8) *Bundle {
	// TODO: more variable data
	bndl, err := Builder().
		CRC(CRC32).
		Source(rapid.StringMatching(DtnEndpointRegexpNotNone).Draw(t, fmt.Sprintf("source %v", i))).
		Destination(rapid.StringMatching(DtnEndpointRegexpFull).Draw(t, fmt.Sprintf("destination %v", i))).
		CreationTimestampNow().
		Lifetime("10m").
		HopCountBlock(64).
		BundleAgeBlock(0).
		PayloadBlock([]byte(rapid.String().Draw(t, fmt.Sprintf("payload %v", i)))).
		Build()
	if err != nil {
		t.Fatalf("Error during bundle creation %s", err)
	}
	return bndl
}
