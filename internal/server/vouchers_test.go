package server

import (
	"context"
	"reflect"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/world"
)

func TestVoucherRefusalsRetainInventory(t *testing.T) {
	a := &assets.Catalog{PetVouchers: map[uint16]uint16{30068: 14719}, Items: map[uint16]game.ItemDefinition{30068: {ID: 30068}}, NPCs: map[uint16]assets.NPC{14719: {ID: 14719}}}
	s := &Server{Assets: a, World: world.New(a)}
	for _, tc := range []struct {
		count  byte
		target uint16
		pets   []game.Pet
	}{
		{2, 0, nil}, {1, 1, nil}, {1, 0, []game.Pet{{ID: 14719, Slot: 1}}},
		{1, 0, []game.Pet{{ID: 1, Slot: 1}, {ID: 2, Slot: 2}, {ID: 3, Slot: 3}, {ID: 4, Slot: 4}}},
	} {
		wire := &captureConn{}
		char := &game.Character{Pets: tc.pets, Quests: map[uint32]game.Quest{}}
		char.Bag[0] = game.Item{ID: 30068, Count: 2}
		before := char.Clone()
		c := &Session{conn: wire, character: char, pets: newPetRoster()}
		handled, err := s.redeemVoucher(context.Background(), c, 1, tc.count, tc.target)
		if !handled || err != nil || !reflect.DeepEqual(before, *char) || len(wire.packets(t)) != 1 {
			t.Fatal("voucher refusal", tc, handled, err)
		}
	}
}

func TestVoucherRedemptionPersistence(t *testing.T) {
	s, players, wires := petFixture(t)
	c, wire := players[0], wires[0]
	s.Assets.PetVouchers = map[uint16]uint16{30068: 14156}
	s.Assets.Items[30068] = game.ItemDefinition{ID: 30068}
	next := c.character.Clone()
	next.Bag[8] = game.Item{ID: 30068, Count: 2}
	ctx := context.Background()
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 96, 9}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if !contains(got, []byte{23, 9, 9, 1}) || !contains(got, []byte{23, 15}) || c.pets.slot(14156) == 0 {
		t.Fatal("voucher packets", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(chars[0].Pets) != 1 || chars[0].Bag[8].Count != 1 {
		t.Fatal("voucher state", err)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 96, 9}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[8].Count != 1 || len(c.character.Pets) != 1 {
		t.Fatal("duplicate consumed voucher")
	}
}
