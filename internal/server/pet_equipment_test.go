package server

import (
	"context"
	"reflect"
	"testing"
	"wonderland-go/internal/game"
)

func TestPetEquipmentAndAllocationPersistence(t *testing.T) {
	s, players, wires := petFixture(t)
	c, wire := players[0], wires[0]
	ctx := context.Background()
	next := c.character.Clone()
	pet := game.NewPet(14156, "Xaolan", 3, s.petTemplate(14156), s.Assets.Items)
	pet.StatPoints = 3
	pet.Equipment[2] = game.Item{ID: 10063, Count: 1, Damage: 7}
	next.Pets = []game.Pet{pet}
	next.Bag[8] = game.Item{ID: 10064, Count: 1, Damage: 4, Metadata: [26]byte{18: 2}}
	s.Assets.Items[10064] = game.ItemDefinition{ID: 10064, EquipSlot: 3, Status: [2]uint16{210, 207}, Values: [2]int32{120, 110}}
	s.Assets.Items[10063] = game.ItemDefinition{ID: 10063, EquipSlot: 3}
	c.pets.register(14156) // client slot 1 differs from persistent party slot 3.
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	do := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wire.packets(t)
	}
	if got := do(23, 17, 1, 9); !contains(got, []byte{23, 23, 1, 9}) {
		t.Fatal(got)
	}
	if c.character.Pets[0].Equipment[2].ID != 10064 || c.character.Bag[8].Damage != 7 {
		t.Fatal("swap lost metadata")
	}
	before := c.character.Clone()
	if got := do(23, 18, 1, 3, 9); len(got) != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("occupied destination accepted")
	}
	if got := do(23, 18, 1, 3, 10); !contains(got, []byte{23, 22, 1, 3, 10}) || c.character.Bag[9].Forge() != 2 {
		t.Fatal("remove", got)
	}
	do(8, 2, 1, 28, 2)
	if c.character.Pets[0].StatPoints != 1 || c.character.Pets[0].Base.Strength != pet.Base.Strength+2 {
		t.Fatal("wrong pet allocated")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || !reflect.DeepEqual(chars[0].Pets, c.character.Pets) || chars[0].Bag[9] != c.character.Bag[9] {
		t.Fatal("state did not persist", err)
	}
}
