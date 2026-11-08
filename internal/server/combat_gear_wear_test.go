package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestCombatGearWearPersistsPlayerAndPetOnce(t *testing.T) {
	s, c, w := battlePetFixture(t)
	run, m := c.battle, c.battle.member(c)
	s.Assets.Items[10002] = game.ItemDefinition{ID: 10002, EquipSlot: 1}
	next := c.character.Clone()
	next.Equipment[0] = game.Item{ID: 10002, Count: 1, Damage: 10}
	next.Pets[0].Equipment[0] = game.Item{ID: 10002, Count: 1, Damage: 20}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	w.Reset()
	c.character.Reborn = true
	c.character.Skills = []game.LearnedSkill{{ID: 10001, Grade: 1, EXP: 37}}
	run.b.GearHits = map[*battle.Fighter][]battle.GearHit{m.self: {{EnemyLevel: 255, Contacts: 2}}, m.pet: {{EnemyLevel: 255, Contacts: 3}}}
	if err := s.commitCombatGearWearWithRoll(run, func(int, int) int { return 0 }); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Equipment[0].Damage != 12 || saved[0].Pets[0].Equipment[0].Damage != 23 {
		t.Fatal(saved, err)
	}
	if !c.character.Reborn || len(c.character.Skills) != 1 || c.character.Skills[0].EXP != 37 {
		t.Fatal("gear transaction overwrote pending round state")
	}
	w.Reset()
	if err := s.commitCombatGearWearWithRoll(run, func(int, int) int { return 0 }); err != nil || w.Len() != 0 {
		t.Fatal("wear applied twice", err)
	}
}

func TestBattleDefeatRevivesAtOneHP(t *testing.T) {
	s, c, _ := battlePetFixture(t)
	run, m := c.battle, c.battle.member(c)
	m.self.HP, m.pet.HP = 0, 0
	m.self.SP, m.pet.SP = 4, 7
	m.pet.Deaths = 1
	c.character.Pets[1].HP = 0
	m.knockedOutPets = map[uint32]bool{c.character.Pets[1].ID: true}
	s.endBattle(run, battle.Defeat)
	settle(t, s, c)
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].HP != 1 || stored[0].SP != 4 || stored[0].Pets[0].HP != 1 || stored[0].Pets[0].SP != 7 || stored[0].Pets[0].Amity != 59 || stored[0].Pets[1].HP != 1 {
		t.Fatal(stored, err)
	}
}

func TestCombatGearWearRejectsStaleEquipmentAndProtectsPvP(t *testing.T) {
	s, c, w := battlePetFixture(t)
	run, m := c.battle, c.battle.member(c)
	s.Assets.Items[10002] = game.ItemDefinition{ID: 10002, EquipSlot: 1}
	next := c.character.Clone()
	next.Equipment[0] = game.Item{ID: 10002, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	// External mutation cannot be overwritten by an already calculated hit.
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(saved *game.Character) error { saved.Equipment[0].Damage = 9; return nil }); err != nil {
		t.Fatal(err)
	}
	w.Reset()
	run.b.GearHits = map[*battle.Fighter][]battle.GearHit{m.self: {{EnemyLevel: 255, Contacts: 1}}}
	if err := s.commitCombatGearWearWithRoll(run, func(int, int) int { return 0 }); err == nil || w.Len() != 0 || c.character.Equipment[0].Damage != 0 {
		t.Fatal("stale wear published", err)
	}
	run.b.PvP = true
	run.b.GearHits = map[*battle.Fighter][]battle.GearHit{m.self: {{EnemyLevel: 255, Contacts: 1}}}
	if err := s.commitCombatGearWearWithRoll(run, func(int, int) int { t.Fatal("PvP rolled wear"); return 0 }); err != nil {
		t.Fatal(err)
	}
	saved, _ := s.Store.Characters(context.Background(), c.account.ID)
	if saved[0].Equipment[0].Damage != 9 {
		t.Fatal("failed transaction changed gear")
	}
}
