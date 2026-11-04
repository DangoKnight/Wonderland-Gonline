package protocol

import (
	"bytes"
	"testing"
)

func TestCollectionTimingCapabilityGolden(t *testing.T) {
	want := []byte{1, 243, 71, 76, 67, 84, 1, 0}
	if !bytes.Equal(CollectionTimingCapability(), want) || !IsCollectionTimingCapability(want) {
		t.Fatal("collection capability wire changed")
	}
	for _, packet := range [][]byte{{1, 243, 1}, {1, 243, 71, 76, 67, 84, 2, 0}, {1, 243, 71, 76, 67, 84, 1, 0, 0}} {
		if IsCollectionTimingCapability(packet) {
			t.Fatal("accepted unsupported capability", packet)
		}
	}
}
