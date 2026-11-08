package protocol

import (
	"bytes"
	"testing"
)

func TestObservedGroundPickupGolden(t *testing.T) {
	want := []byte{23, 2, 7, 0, 4, 3, 2, 1, 232, 3, 208, 7}
	pickup := ObservedGroundPickup{Slot: 7, Collector: 0x01020304, X: 1000, Y: 2000}
	if !bytes.Equal(pickup.Packet(), want) {
		t.Fatal("wire changed")
	}
	if got, ok := ParseObservedGroundPickup(want); !ok || got != pickup {
		t.Fatal(got, ok)
	}
	for n := range len(want) {
		if _, ok := ParseObservedGroundPickup(want[:n]); ok {
			t.Fatal("truncation accepted", n)
		}
	}
	if _, ok := ParseObservedGroundPickup(append(append([]byte(nil), want...), 0)); ok {
		t.Fatal("trailing bytes accepted")
	}
}
