package world

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
)

func TestInstalledBeachGroundDepth(t *testing.T) {
	w := nativeWorld(t)
	// Complete native map-entry packet captured from the real server, 2026-11-03.
	want := []byte{23, 4, 3, 1, 0, 106, 160, 0, 0, 174, 5, 103, 3, 160, 0, 0, 0, 3, 2, 0, 106, 160, 0, 0, 206, 1, 26, 2, 170, 0, 0, 0}
	if got := w.GroundPacket(11016); !bytes.Equal(got, want) {
		t.Fatalf("installed Beach item definitions differ: %v", got)
	}
}

func TestNativeGroundDepthGoldenAndRespawn(t *testing.T) {
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{11016: {ID: 11016, Items: []assets.GroundItem{
		{ClickID: 1, ItemID: 41066, X: 1454, Y: 871, Unknown: [2]uint16{160, 0}},
		{ClickID: 2, ItemID: 41066, X: 462, Y: 538, Unknown: [2]uint16{170, 0}},
	}}}})
	want := []byte{23, 4, 3, 1, 0, 106, 160, 0, 0, 174, 5, 103, 3, 160, 0, 0, 0, 3, 2, 0, 106, 160, 0, 0, 206, 1, 26, 2, 170, 0, 0, 0}
	if got := w.GroundPacket(11016); !bytes.Equal(got, want) {
		t.Fatalf("native depth missing: %v", got)
	}
	target, ok := w.GroundAt(11016, 2)
	if !ok {
		t.Fatal("left coconut missing")
	}
	now := time.Unix(1000, 0)
	w.Take(11016, target, now)
	if got := w.Respawn(now.Add(119 * time.Second)); len(got) != 0 {
		t.Fatal("draw depth altered respawn timing")
	}
	got := w.Respawn(now.Add(120 * time.Second))[11016]
	if len(got) != 1 || !bytes.Equal(got[0], []byte{23, 4, 3, 2, 0, 106, 160, 0, 0, 206, 1, 26, 2, 170, 0, 0, 0}) {
		t.Fatal("respawn lost native depth", got)
	}
}
