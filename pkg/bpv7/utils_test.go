package bpv7

import (
	"testing"
)

// Kept local instead of using internal/testhelpers.SampleBundle: the tests in
// this package cannot import it, as it imports this package.
func generateSampleBundle(t *testing.T) *Bundle {
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
