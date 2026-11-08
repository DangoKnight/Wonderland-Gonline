package world

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestAccountGroundFilteringLeavesPlayerDropsShared(t *testing.T) {
	catalog := &assets.Catalog{Maps: map[uint16]assets.Map{12345: {ID: 12345, Items: []assets.GroundItem{{ClickID: 7, ItemID: 30001, X: 10, Y: 20}}}}, Story: assets.StoryDefinitions{Rules: []assets.StoryRule{{ID: "personal", Kind: assets.StoryAccountGroundOnce, Scope: assets.StoryScope{Maps: []uint16{12345}}, Value: 12345}}}}
	w := New(catalog)
	target, ok := w.GroundAt(12345, 7)
	if !ok {
		t.Fatal("missing source")
	}
	w.Take(12345, target, time.Now())
	if _, ok := w.GroundAt(12345, 7); !ok {
		t.Fatal("account claim changed global availability")
	}
	w.Drop(12345, []byte{1}, game.Item{ID: 30002, Count: 1}, 10, 20)
	want := []byte{23, 4, 3, 1, 0, 50, 117, 0, 0, 10, 0, 20, 0, 0, 0, 0, 0}
	if got := w.GroundPacket(12345, map[byte]bool{7: true}); !bytes.Equal(got, want) {
		t.Fatal("claimed native node or player drop filtered incorrectly", got)
	}
	drop, ok := w.GroundAt(12345, 1)
	if !ok || drop.Native() {
		t.Fatal("drop mistaken for personal source")
	}
	w.Take(12345, drop, time.Now())
	if _, ok := w.GroundAt(12345, 1); ok {
		t.Fatal("player drop was not consumed")
	}
	if got := w.GroundPacket(12345, map[byte]bool{7: true}); got != nil {
		t.Fatal("claimed node still visible", got)
	}
}
