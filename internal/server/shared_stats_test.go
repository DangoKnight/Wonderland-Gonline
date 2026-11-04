package server

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestBattleCatalogStatsReachEnemiesAndOwnedPets(t *testing.T) {
	s, sessions, _ := worldFixture(t)
	c := sessions[0]
	if s.Assets.NPCs == nil {
		s.Assets.NPCs = map[uint16]assets.NPC{}
	}
	s.Assets.NPCs[17411] = assets.NPC{ID: 17411, Level: 4, HP: 40, SP: 0, Element: game.Wind, Stats: [5]uint16{2, 2, 1, 1, 2}}
	// Persisted pets carry ownership and attributes, not copied NPC definitions.
	c.character.Pets = []game.Pet{{ID: 17411, Level: 4, Slot: 1, Amity: 60, HP: 194, SP: 105, Base: game.Attributes{Strength: 2, Constitution: 2, Intelligence: 1, Wisdom: 1, Agility: 2}}}
	c.character.ActivePet = 17411
	c.pets.register(17411)
	if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 17411, Level: 4}}); err != nil {
		t.Fatal(err)
	}
	run := c.battle
	for _, f := range []*battle.Fighter{run.members[0].pet, run.b.Defenders[0]} {
		if f == nil || f.Element != game.Wind || f.Def != 12 || f.Spd != 12 {
			t.Fatalf("catalog element/attributes did not reach formula: %+v", f)
		}
	}
	pet, enemy := run.members[0].pet, run.b.Defenders[0]
	if pet.MaxHP != 194 || pet.MaxSP != 105 || enemy.MaxHP != 40 || enemy.HP != 40 || enemy.MaxSP != 0 || enemy.SP != 0 {
		t.Fatalf("pet formulas or stored enemy vitals changed: pet %+v, enemy %+v", pet, enemy)
	}
}

func TestBattleStoredEnemyVitalsAndEncounterOverrides(t *testing.T) {
	s, sessions, _ := worldFixture(t)
	s.Assets.NPCs = map[uint16]assets.NPC{
		17000: {ID: 17000, Level: 1, HP: 36, SP: 100},
		17002: {ID: 17002, Level: 1, HP: 40, SP: 0},
	}
	c := sessions[0]
	enemies := make([]battle.Enemy, 2*battle.MaxEnemies+1)
	for i := range enemies {
		enemies[i] = battle.Enemy{Template: 17000, Level: 1}
	}
	enemies[battle.MaxEnemies] = battle.Enemy{Template: 17002, Level: 20}
	enemies[2*battle.MaxEnemies].HP = 700
	if err := s.startBattle(c, &battleRun{}, enemies); err != nil {
		t.Fatal(err)
	}
	b := c.battle.b
	f := b.Defenders[0]
	if f.HP != 36 || f.MaxHP != 36 || f.SP != 100 || f.MaxSP != 100 {
		t.Fatalf("initial enemy did not use stored vitals: %+v", f)
	}
	waves := b.Reinforcements[0]
	if len(waves) != 2 || waves[0].HP != 40 || waves[0].SP != 0 || waves[1].HP != 700 || waves[1].SP != 100 {
		t.Fatalf("reinforcement vitals or encounter override lost: %+v", waves)
	}
}
