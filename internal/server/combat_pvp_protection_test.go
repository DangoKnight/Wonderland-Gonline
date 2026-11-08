package server

import (
	"context"
	"testing"

	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestPvPProtectsPetAmityAndEquipmentAtEveryCheckpoint(t *testing.T) {
	for _, scenario := range []string{"victory", "defeat", "escape", "switch", "disconnect"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, _ := battlePetFixture(t)
			run, m := c.battle, c.battle.member(c)
			run.b.PvP = true
			next := c.character.Clone()
			next.Pets[0].Amity = game.PetDesertMinimumAmity
			next.Equipment[game.CaptureNetEquipmentSlot-1] = game.Item{ID: game.MonsterNetItemID, Count: 1, Damage: 30}
			next.Pets[0].Equipment[0] = game.Item{ID: 10002, Count: 1, Damage: 40}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			m.pet.HP, m.pet.SP, m.pet.Deaths = 0, 7, 3
			if scenario == "disconnect" {
				s.stageBattleDisconnect(run, m)
				if err := s.saveBattleDisconnect(context.Background(), c); err != nil {
					t.Fatal(err)
				}
			} else {
				next = c.character.Clone()
				outcome := map[string]battle.Outcome{"victory": battle.Victory, "defeat": battle.Defeat, "escape": battle.Fled, "switch": battle.Continue}[scenario]
				s.activePetResults(run, m, &next, outcome, c.pets)
				if err := s.commit(context.Background(), c, next); err != nil {
					t.Fatal(err)
				}
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil {
				t.Fatal(err)
			}
			got := stored[0]
			if len(got.Pets) != len(before.Pets) || got.Pets[0].Amity != before.Pets[0].Amity || got.ActivePet != before.ActivePet {
				t.Fatal("PvP knockout penalized/deserted pet", got.Pets)
			}
			if got.Equipment != before.Equipment || got.Pets[0].Equipment != before.Pets[0].Equipment {
				t.Fatal("PvP damaged equipment")
			}
			wantHP := int32(1)
			if scenario == "switch" || scenario == "disconnect" {
				wantHP = 0
			}
			if got.Pets[0].HP != wantHP || got.Pets[0].SP != 7 {
				t.Fatal("protection discarded resolved vitals", got.Pets[0])
			}
		})
	}
}

func TestPvPCaptureCannotWearEquipment(t *testing.T) {
	s, c, wire := battlePetFixture(t)
	run := c.battle
	run.b.PvP = true
	next := c.character.Clone()
	next.Equipment[game.CaptureNetEquipmentSlot-1] = game.Item{ID: game.MonsterNetItemID, Count: 1, Damage: 30}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if _, ok := s.combatRules(run).PrepareCapture(battle.Action{Player: c.character.ID}); ok {
		t.Fatal("PvP capture allowed")
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].Equipment != next.Equipment || wire.Len() != 0 {
		t.Fatal("PvP capture changed equipment", err)
	}
}
