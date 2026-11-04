package battle

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestEnemyAndPetUsePlayerElementalStats(t *testing.T) {
	for _, element := range []byte{game.Earth, game.Water, game.Fire, game.Wind} {
		base := game.Attributes{Strength: 5, Constitution: 5, Intelligence: 7, Wisdom: 9, Agility: 11}
		c := game.Character{ID: 10001, Level: 10, Element: element, Base: base}
		c.Refill(nil)
		// Formula parity requires identical owned attributes; recruitment now
		// intentionally applies initial-stat ranges or the half-template rule.
		p := game.Pet{ID: 17411, Name: "Floating", Slot: 1, Level: 10, Base: base}
		p.Normalize(nil, true, game.PetTemplate{Element: element})
		player := PlayerFighter(&c, nil, 0)
		pet := PetFighter(p, c.ID, nil, 0, game.PetTemplate{Element: element})
		b := New([]*Fighter{player, pet}, []Enemy{{Template: 17411, Level: 10, Element: element, Base: base}})
		stats := func(f *Fighter) [7]int { return [7]int{f.MaxHP, f.MaxSP, f.Atk, f.Def, f.Matk, f.Mdef, f.Spd} }
		if stats(player) != stats(pet) || stats(player) != stats(b.Defenders[0]) || pet.Element != element {
			t.Fatalf("element %d: player %v, pet %v, enemy %v", element, stats(player), stats(pet), stats(b.Defenders[0]))
		}
	}
}

func TestEnemyExplicitOverridesPreserveOtherPlayerStats(t *testing.T) {
	b := New(nil, []Enemy{{Level: 10, Element: game.Fire, HP: 12345, Attack: 678}})
	f := b.Defenders[0]
	if f.MaxHP != 12345 || f.HP != 12345 || f.Atk != 678 || f.Matk != 678 || f.Def != 20 || f.Mdef != 20 || f.Spd != 16 || f.SP != 104 {
		t.Fatalf("scripted enemy overrides: %+v", f)
	}
}
