package server

import (
	"context"
	"testing"
)

func TestNativeHotbarAssignmentDoesNotSynthesize(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	before := c.character.Clone()
	// Independent native golden bytes: AC40:1, skill 10001, page 1/F1.
	// Item binding uses ID 100; zero ID removes a binding locally.
	for _, p := range [][]byte{
		{40, 1, 2, 0x11, 0x27, 1, 1}, {40, 1, 2, 0x75, 0xea, 3, 8},
		{40, 1, 1, 100, 0, 2, 5}, {40, 1, 2, 0, 0, 1, 1},
	} {
		if err := s.dispatch(context.Background(), c, p); err != nil {
			t.Fatalf("native binding %x: %v", p, err)
		}
		if c.character.Bag != before.Bag || c.character.Gold != before.Gold {
			t.Fatal("binding modified resources")
		}
		for _, w := range wires {
			if w.Len() != 0 {
				t.Fatal("binding emitted an incompatible alchemy reply")
			}
		}
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag != before.Bag || chars[0].Gold != before.Gold {
		t.Fatal("binding modified durable resources", err)
	}
	// Hotbar preferences are also safe during map loading and trade gates.
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{40, 1, 2, 0x11, 0x27, 1, 1}); err != nil {
		t.Fatal(err)
	}
}
func TestNativeHotbarMalformed(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	before := c.character.Bag
	for _, p := range [][]byte{
		{40, 1, 2, 0x11, 0x27, 1}, {40, 1, 2, 0x11, 0x27, 1, 1, 0},
		{40, 1, 3, 0x11, 0x27, 1, 1}, {40, 1, 2, 0x11, 0x27, 0, 1},
		{40, 1, 2, 0x11, 0x27, 4, 1}, {40, 1, 2, 0x11, 0x27, 1, 0},
		{40, 1, 2, 0x11, 0x27, 1, 9}, {40, 2, 2, 0x11, 0x27, 1, 1},
	} {
		if err := s.dispatch(context.Background(), c, p); err == nil {
			t.Fatalf("malformed binding accepted: %x", p)
		}
		if c.character.Bag != before || wires[0].Len() != 0 {
			t.Fatal("malformed binding mutated state")
		}
	}
}
