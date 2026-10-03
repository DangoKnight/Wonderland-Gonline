package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func compoundFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := worldFixture(t)
	for _, id := range []uint16{100, 200, 300} {
		s.Assets.Items[id] = game.ItemDefinition{ID: id, Type: 23}
	}
	s.Assets.AlchemyRecipes = []assets.AlchemyRecipe{{Input1: 100, Input2: 200, Output: 300}}
	for _, c := range players {
		if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	next := players[0].character.Clone()
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 100, Count: 1}
	next.Bag[8] = game.Item{ID: 200, Count: 1}
	if err := s.commit(context.Background(), players[0], next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestCompoundNativePacketsPersistenceAndReplay(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	// Exercise dispatch, not only the item handler.
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 9, 5}); err != nil {
		t.Fatal(err)
	}
	result := append([]byte{23, 8, 5, 44, 1, 1}, make([]byte, 28)...)
	animation := protocol.Builder{23, 122}.U32(c.character.ID)
	want := [][]byte{{23, 9, 9, 1}, {23, 9, 5, 1}, result, {23, 13, 44, 1, 1, 5}, animation}
	got := wires[0].packets(t)
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d: %v != %v", i, got[i], want[i])
		}
	}
	peer := wires[1].packets(t)
	if len(peer) != 1 || !bytes.Equal(peer[0], animation) || wires[2].Len() != 0 {
		t.Fatal("animation map isolation", peer)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Bag[4] != (game.Item{ID: 300, Count: 1}) || !chars[0].Bag[8].Empty() {
		t.Fatal("not persisted", err)
	}
	before := c.character.Bag
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 9, 5}); err != nil || c.character.Bag != before || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("empty-slot replay changed state", err)
	}
}

func TestCompoundRefusesMalformedAndUnavailableInputs(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	before := c.character.Bag
	for _, p := range [][]byte{{23, 14}, {23, 14, 2, 5}, {23, 14, 2, 5, 9, 1}, {23, 14, 3, 5, 9}, {23, 14, 2, 5, 5}, {23, 14, 2, 0, 9}, {23, 14, 2, 51, 9}, {23, 14, 2, 1, 9}} {
		err := s.worldCommand(context.Background(), c, p)
		if len(p) != 5 && err == nil {
			t.Fatalf("truncated or extra packet accepted: %v", p)
		}
		if c.character.Bag != before || wires[0].Len() != 0 || wires[1].Len() != 0 {
			t.Fatal("invalid input changed state", p)
		}
	}
	delete(s.Assets.Items, 300)
	if err := s.worldCommand(context.Background(), c, []byte{23, 14, 2, 5, 9}); err != nil || c.character.Bag != before || wires[0].Len() != 0 {
		t.Fatal("unknown output", err)
	}
	s.Assets.AlchemyRecipes = nil
	// No recipe uses the same deterministic greater-ID fallback as native AC23.
	if err := s.worldCommand(context.Background(), c, []byte{23, 14, 2, 5, 9}); err != nil || c.character.Bag[4].ID != 200 {
		t.Fatal("fallback", err)
	}
}

func TestCompoundFullBagAndFailedSave(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 100, Count: 2, Damage: 5, Metadata: [26]byte{8}}
	}
	next.Bag[8].ID = 200
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Bag
	if err := s.worldCommand(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil || before != c.character.Bag || wires[0].Len() != 0 {
		t.Fatal("full bag consumed ingredients", err)
	}
	// A removal can free the other ingredient slot even in a full bag.
	next = c.character.Clone()
	next.Bag[8].Count = 1
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before = c.character.Bag
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.worldCommand(canceled, c, []byte{23, 14, 2, 5, 9}); err == nil || c.character.Bag != before || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("save failure published success", err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != before {
		t.Fatal("failed save mutated persistence", err)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil || c.character.Bag[8] != (game.Item{ID: 300, Count: 1}) || c.character.Bag[4].Count != 1 || c.character.Bag[4].Metadata != before[4].Metadata {
		t.Fatal("leftovers", err)
	}
}

func TestCompoundOwnershipGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "event", "storm", "beach", "trade", "mounted"} {
		t.Run(gate, func(t *testing.T) {
			s, players, wires := compoundFixture(t)
			c := players[0]
			before := c.character.Bag
			switch gate {
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "event":
				c.event = &eventSession{}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			case "trade":
				c.trade = &tradeSession{}
			case "mounted":
				c.character.ActiveVehicle = 100
				c.character.VehicleSlot = 5
			}
			err := s.worldCommand(context.Background(), c, []byte{23, 14, 2, 5, 9})
			if gate == "loading" && err == nil {
				t.Fatal("pre-ack accepted")
			}
			if c.character.Bag != before || wires[1].Len() != 0 || wires[2].Len() != 0 {
				t.Fatal("reserved items changed", err)
			}
			packets := wires[0].packets(t)
			if gate == "trade" {
				if len(packets) != 1 || packets[0][1] != 57 {
					t.Fatal("trade warning", packets)
				}
			} else if len(packets) != 0 {
				t.Fatal("gated success", packets)
			}
		})
	}
}
