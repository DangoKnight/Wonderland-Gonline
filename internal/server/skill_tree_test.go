package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestAllocationUnlockCommitsWithStats(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	s.Assets.Skills[15101] = assets.Skill{ID: 15101}
	c.character.StatPoints = 1
	c.character.Base.Strength = 15 - game.CharacterBonuses(c.character.Body, c.character.Head).Strength
	beforeHP, beforeSP := c.character.HP, c.character.SP
	if err := s.worldCommand(ctx, c, []byte{8, game.StatSTR, 1, 0}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].StatPoints != 0 {
		t.Fatal(stored, err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 15101 && sk.Grade == 1 {
			found = true
		}
	}
	if !found || !contains(wires[0].packets(t), protocol.Builder{5, 12}.U16(15101).U8(1)) {
		t.Fatal("unlock not persisted/sent")
	}
	if c.character.HP != beforeHP || c.character.SP != beforeSP {
		t.Fatal("allocation refilled HP/SP")
	}
}

func TestCombatEvolutionCommitsWithGrade(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills[15104] = assets.Skill{ID: 15104}
	c.character.Skills = []game.LearnedSkill{{ID: 11166, Grade: 9, EXP: 899}}
	packets, err := s.commitSkillUses(c, c.battle, []battle.Action{{Actor: c.battle.members[0].self, Skill: 11166}})
	if err != nil || !contains(packets, protocol.Builder{5, 12}.U16(15104).U8(1)) {
		t.Fatal(packets, err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 15104 && sk.Grade == 1 {
			found = true
		}
	}
	if stored[0].Skills[0].Grade != 10 || !found {
		t.Fatal(stored[0].Skills)
	}
}

func TestLoginUnlockIsPersistedBeforeSnapshot(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	next.Base.Strength = 16 - game.CharacterBonuses(next.Body, next.Head).Strength
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	s.Assets.Skills[15101] = assets.Skill{ID: 15101, TableOrder: 900}
	c.character = nil
	wires[0].Reset()
	if err := s.dispatch(ctx, c, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 15101 {
			found = true
		}
	}
	if !found || c.character == nil {
		t.Fatal("login unlock missing")
	}
	// Snapshot only: no allocation packets are allowed to overwrite login stats.
	if contains(wires[0].packets(t), protocol.Builder{8, 1, 110, 1}.U32(1).U32(15101)) {
		t.Fatal("incremental unlock sent during login")
	}
}

func TestAllocationUnlockSaveFailureHasNoSuccess(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	s.Assets.Skills[15101] = assets.Skill{ID: 15101}
	c.character.StatPoints = 1
	c.character.Base.Strength = 15 - game.CharacterBonuses(c.character.Body, c.character.Head).Strength
	before := c.character.Base.Strength
	s.Store.Close()
	if err := s.worldCommand(ctx, c, []byte{8, game.StatSTR, 1, 0}); err == nil {
		t.Fatal("save failure ignored")
	}
	if c.character.Base.Strength != before || c.character.StatPoints != 1 || wires[0].Len() != 0 {
		t.Fatal("failed allocation changed state or sent success")
	}
}
