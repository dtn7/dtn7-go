package dummy_cla

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dtn7/dtn7-ng/pkg/bpv7"
	"pgregory.net/rapid"
)

func TestSendReceive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numberOfBundles := rapid.IntRange(1, 1000).Draw(t, "Number of Bundles")
		var wgSend sync.WaitGroup
		wgSend.Add(numberOfBundles)
		var wgReceive sync.WaitGroup
		wgReceive.Add(numberOfBundles)

		receiveFunc := func(bundle bpv7.Bundle) (interface{}, error) {
			wgReceive.Done()
			return nil, nil
		}

		peerIDA := bpv7.MustNewEndpointID(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(t, "peerA"))
		peerIDB := bpv7.MustNewEndpointID(rapid.StringMatching(bpv7.DtnEndpointRegexpNotNone).Draw(t, "peerB"))
		peerA, peerB := NewDummyCLAPair(peerIDA, peerIDB, receiveFunc)
		_ = peerA.Activate()
		_ = peerB.Activate()
		peers := []*DummyCLA{peerA, peerB}

		bundles := make([]bpv7.Bundle, numberOfBundles)
		for i := 0; i < numberOfBundles; i++ {
			bundles[i] = bpv7.GenerateBundle(t, i)
		}

		for i := 0; i < numberOfBundles; i++ {
			sender := peers[rapid.IntRange(0, len(peers)-1).Draw(t, fmt.Sprintf("Sender %v", i))]
			go func(i int, sender *DummyCLA) {
				bundle := bundles[i]
				err := sender.Send(bundle)
				wgSend.Done()
				if err != nil {
					t.Fatal(err)
				}
			}(i, sender)
		}
		wgSend.Wait()
		wgReceive.Wait()

		_ = peerA.Close()
		time.Sleep(time.Millisecond)
		_ = peerB.Close()
	})
}
