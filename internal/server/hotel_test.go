package server

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func TestDecodeHotel(t *testing.T) {
	for _, p := range [][]byte{{31}, {31, 1}, {31, 2}, {31, 3, 1, 1}, {31, 2, 11}, {31, 3, 5}, {31, 4, 1}, {31, 4, 1, 1, 1}, {31, 4, 1, 1, 0, 9}, {31, 4, 0, 0}} {
		if _, _, ok := decodeHotel(p); ok {
			t.Fatalf("accepted %v", p)
		}
	}
	d, w, ok := decodeHotel([]byte{31, 4, 2, 4, 1, 2, 10, 3})
	if !ok || !bytes.Equal(d, []byte{4, 1}) || !bytes.Equal(w, []byte{10, 3}) {
		t.Fatal(d, w, ok)
	}
}

func TestHotelExchangeAndPersistence(t *testing.T) {
	s, players, wires := petFixture(t)
	c, wire := players[0], wires[0]
	ctx := context.Background()
	next := c.character.Clone()
	for i := 0; i < 4; i++ {
		next.Pets = append(next.Pets, game.Pet{ID: uint32(14000 + i), Slot: byte(i + 1), Name: "Team", Level: 2, Exp: 10, Amity: 80, Skills: []game.PetSkill{{ID: 11001, Grade: 2, Exp: 99}}})
		c.pets.register(uint32(14000 + i))
	}
	for i := 0; i < 10; i++ {
		next.HotelPets = append(next.HotelPets, game.Pet{ID: uint32(15000 + i), Slot: byte(i + 1), Name: "Hotel", Level: 3})
	}
	next.Pets[0].Battle = true
	next.ActivePet = 14000
	next.Pets[0].Equipment[2] = game.Item{ID: 10063, Count: 1, Damage: 4}
	outgoing := next.Pets[0]
	outgoing.Battle = false
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	for _, p := range [][]byte{{31, 3, 1}, {31, 2, 1}, {31, 4, 1, 1, 1, 11}} {
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, *c.character) || len(wire.packets(t)) != 0 {
			t.Fatal("rejected request mutated state")
		}
	}
	if err := s.worldCommand(ctx, c, []byte{31, 4, 1, 1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if !contains(got, []byte{31, 3, 1, 1}) || !contains(got, protocol.Builder{15, 2}.U32(next.ID).U8(1)) || !contains(got, []byte{19, 2}) {
		t.Fatal("missing exchange packets", got)
	}
	if c.character.ActivePet != 0 || c.pets.slot(14000) != 0 || c.pets.slot(15000) != 1 {
		t.Fatal("active pet or client slots")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored := chars[0]
	if len(stored.Pets) != 4 || len(stored.HotelPets) != 10 {
		t.Fatal("exchange capacity")
	}
	found := false
	for _, pet := range stored.HotelPets {
		if pet.ID == outgoing.ID {
			found = true
			if !reflect.DeepEqual(pet, outgoing) {
				t.Fatal("pet metadata lost", pet, outgoing)
			}
		}
	}
	if !found {
		t.Fatal("deposited pet missing")
	}
}
