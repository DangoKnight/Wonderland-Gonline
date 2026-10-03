package world

import (
	"bytes"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func groundFixture() *World {
	m := assets.Map{ID: 12345, Items: []assets.GroundItem{
		{ClickID: 7, ItemID: 30001, X: 10, Y: 20, Unknown: [2]uint16{30, 0}},
		{ClickID: 300, ItemID: 30002, X: 11, Y: 21}, // Slot 2 by running index.
	}}
	return New(&assets.Catalog{Maps: map[uint16]assets.Map{m.ID: m}})
}

func TestGroundPickupRespawn(t *testing.T) {
	w := groundFixture()
	now := time.Unix(1000, 0)
	target, ok := w.GroundAt(12345, 7)
	if !ok || target.Item != (game.Item{ID: 30001, Count: 1}) || !target.InRange(10+180, 20) || target.InRange(10+128, 20+128) {
		t.Fatal(target, ok)
	}
	w.Take(12345, target, now)
	if _, ok := w.GroundAt(12345, 7); ok {
		t.Fatal("taken item offered again")
	}
	if p := w.GroundPacket(12345); len(p) != 2+15 || p[3] != 2 {
		t.Fatalf("taken item still listed: %v", p)
	}
	if got := w.Respawn(now.Add(29 * time.Second)); len(got) != 0 {
		t.Fatal("respawned early")
	}
	got := w.Respawn(now.Add(30 * time.Second))
	if len(got[12345]) != 1 || !bytes.Equal(got[12345][0], []byte{23, 4, 3, 7, 0, 0x31, 0x75, 0, 0, 10, 0, 20, 0, 0, 0, 0, 0}) {
		t.Fatalf("respawn: %v", got)
	}
	if _, ok := w.GroundAt(12345, 7); !ok {
		t.Fatal("respawned item unavailable")
	}
}

func TestGroundDropSlots(t *testing.T) {
	w := groundFixture()
	// Slot 2 (index), 7 (slot/click) are native; 300 cannot collide with a byte slot.
	slots, ok := w.FreeGroundSlots(12345, 3)
	if !ok || !bytes.Equal(slots, []byte{1, 3, 4}) {
		t.Fatal(slots, ok)
	}
	item := game.Item{ID: 21001, Count: 3, Damage: 4}
	p := w.Drop(12345, slots, item, 100, 200)
	if len(p) != 2+3*15 {
		t.Fatalf("drop packet: %v", p)
	}
	target, ok := w.GroundAt(12345, 3)
	if !ok || target.Item != (game.Item{ID: 21001, Count: 1, Damage: 4}) || target.X != 100 {
		t.Fatal("dropped unit", target)
	}
	w.Take(12345, target, time.Now())
	if _, ok := w.GroundAt(12345, 3); ok {
		t.Fatal("dropped item picked twice")
	}
	if next, _ := w.FreeGroundSlots(12345, 1); next[0] != 3 {
		t.Fatal("picked slot not reusable")
	}
	if _, ok := w.FreeGroundSlots(12345, 254); ok {
		t.Fatal("more slots than exist")
	}
}
