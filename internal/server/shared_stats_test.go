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
	s.Assets.NPCs[17411] = assets.NPC{ID: 17411, Level: 4, HP: 99999, SP: 99999, Element: game.Wind, Stats: [5]uint16{2, 2, 1, 1, 2}}
	// Persisted pets carry ownership and attributes, not copied NPC definitions.
	c.character.Pets = []game.Pet{{ID: 17411, Level: 4, Slot: 1, Amity: 60, HP: 194, SP: 105, Base: game.Attributes{Strength: 2, Constitution: 2, Intelligence: 1, Wisdom: 1, Agility: 2}}}
	c.character.ActivePet = 17411
	c.pets.register(17411)
	if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 17411, Level: 4}}); err != nil {
		t.Fatal(err)
	}
	run := c.battle
	for _, f := range []*battle.Fighter{run.members[0].pet, run.b.Defenders[0]} {
		if f == nil || f.Element != game.Wind || f.Def != 12 || f.Spd != 12 || f.MaxHP != 194 || f.MaxSP != 105 {
			t.Fatalf("catalog element/attributes did not reach formula: %+v", f)
		}
	}
}
