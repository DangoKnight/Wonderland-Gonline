package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestPetInitialStatsCompanionAliasesPreserveNPCDefinition(t *testing.T) {
	definition := game.PetInitialStats{Minimum: game.Attributes{Strength: 4, Constitution: 5, Intelligence: 2, Wisdom: 2, Agility: 4}, Maximum: game.Attributes{Strength: 5, Constitution: 6, Intelligence: 3, Wisdom: 3, Agility: 5}, TotalPoints: 18}
	c := &assets.Catalog{NPCs: map[uint16]assets.NPC{14161: {ID: 14161, Stats: [5]uint16{9, 10, 4, 5, 8}}, 14162: {ID: 14162, Stats: [5]uint16{9, 10, 4, 5, 8}}}, PetInitialStats: map[uint32]game.PetInitialStats{14162: definition}}
	w := New(c)
	for _, id := range []uint32{14161, 14162} {
		tpl, ok := w.Template(id)
		if !ok || tpl.Stats != (game.Attributes{Strength: 9, Constitution: 10, Intelligence: 4, Wisdom: 5, Agility: 8}) || tpl.InitialStats == nil || *tpl.InitialStats != definition {
			t.Fatal("incorrect companion alias projection", id, tpl)
		}
	}
}
