package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestFailedBattleResultDoesNotPublishPetChanges(t *testing.T) {
	for _, capture := range []bool{false, true} {
		s, c, wire := battleFixture(t, 1000)
		run := c.battle
		if capture {
			run.b.Captures = []battle.Capture{{Owner: c.character.ID, Template: 10500, Name: "Slime", Level: 5}}
		} else {
			pet := game.Pet{ID: 10501, Slot: 1, Amity: 20, HP: 10, MaxHP: 10}
			c.character.Pets = []game.Pet{pet}
			c.character.ActivePet = pet.ID
			c.character.ActiveMount = pet.ID
			c.pets.register(pet.ID)
			run.members[0].pet = battle.PetFighter(pet, c.character.ID, nil, 0)
			run.members[0].pet.Deaths = 1
		}
		wire.Reset()
		before := c.character.Clone()
		s.Store.Close()
		s.endBattle(run, battle.Victory)
		if c.battle != nil || wire.Len() != 0 || len(c.character.Pets) != len(before.Pets) || c.character.ActivePet != before.ActivePet || c.character.ActiveMount != before.ActiveMount {
			t.Fatal("failed result published or adopted pet state")
		}
		if capture && len(c.pets.slots) != 0 {
			t.Fatal("failed capture registered client slot")
		}
		if !capture && c.pets.slot(10501) != 1 {
			t.Fatal("failed desertion removed client slot")
		}
	}
}

func TestCommittedDesertionAdoptsRoster(t *testing.T) {
	s, c, wire := battleFixture(t, 1000)
	pet := game.Pet{ID: 10501, Slot: 1, Amity: 20, HP: 10, MaxHP: 10}
	c.character.Pets = []game.Pet{pet}
	c.character.ActivePet = pet.ID
	c.character.ActiveMount = pet.ID
	c.pets.register(pet.ID)
	run := c.battle
	run.members[0].pet = battle.PetFighter(pet, c.character.ID, nil, 0)
	run.members[0].pet.Deaths = 1
	wire.Reset()
	s.endBattle(run, battle.Defeat)
	settle(t, s, c)
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored[0].Pets) != 0 || stored[0].ActivePet != 0 || stored[0].ActiveMount != 0 || c.pets.slot(pet.ID) != 0 {
		t.Fatal("desertion not durably adopted", err)
	}
	if !contains(wire.packets(t), []byte{19, 2}) {
		t.Fatal("committed desertion not sent")
	}
}
