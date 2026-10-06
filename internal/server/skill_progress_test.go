package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestBattleSkillProgressIsDurable(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	c.character.Skills = []game.LearnedSkill{{ID: 11001, Grade: 1, EXP: 99}}
	run := c.battle
	packets, err := s.commitSkillUses(c, run, []battle.Action{{Actor: run.members[0].self, Skill: 11001}})
	if err != nil || !contains(packets, protocol.Builder{8, 1, 110, 1}.U32(2).U32(11001)) {
		t.Fatal(packets, err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored) == 0 || stored[0].Skills[0].Grade != 2 || stored[0].Skills[0].EXP != 0 {
		t.Fatal(stored, err)
	}
	packets, err = s.commitSkillUses(c, run, []battle.Action{{Actor: run.members[0].self, Skill: 65535}})
	if err != nil || len(packets) != 0 {
		t.Fatal("unlearned action progressed", packets, err)
	}
}

func TestBattleSkillSaveFailurePreservesSession(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	c.character.Skills = []game.LearnedSkill{{ID: 11001, Grade: 1, EXP: 99}}
	s.Store.Close()
	packets, err := s.commitSkillUses(c, c.battle, []battle.Action{{Actor: c.battle.members[0].self, Skill: 11001}})
	if err == nil || len(packets) != 0 || c.character.Skills[0].Grade != 1 || c.character.Skills[0].EXP != 99 {
		t.Fatal(packets, err, c.character.Skills)
	}
}

func TestBattlePetSkillProgressWithoutClientSlot(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	pet := game.Pet{ID: 10501, Skills: []game.PetSkill{{ID: 11001, Grade: 9, Exp: 899}}}
	c.character.Pets = []game.Pet{pet}
	run := c.battle
	run.members[0].pet = battle.PetFighter(pet, c.character.ID, nil, 0)
	packets, err := s.commitSkillUses(c, run, []battle.Action{{Actor: run.members[0].pet, Skill: 11001}})
	if err != nil || len(packets) != 0 || c.character.Pets[0].Skills[0].Grade != 10 || run.members[0].pet.Pet.Skills[0].Grade != 10 {
		t.Fatal(packets, err, c.character.Pets)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].Pets[0].Skills[0].Grade != 10 || stored[0].Pets[0].Skills[0].Exp != 0 {
		t.Fatal(stored, err)
	}
}
