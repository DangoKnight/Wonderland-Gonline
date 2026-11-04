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
		b := New([]*Fighter{player, pet}, []Enemy{{Template: 17411, Level: 10, Element: element, Base: base, HP: 36, SP: 100}})
		stats := func(f *Fighter) [7]int { return [7]int{f.MaxHP, f.MaxSP, f.Atk, f.Def, f.Matk, f.Mdef, f.Spd} }
		enemy := b.Defenders[0]
		wantEnemy := stats(player)
		wantEnemy[0], wantEnemy[1] = 36, 100
		if stats(player) != stats(pet) || wantEnemy != stats(enemy) || pet.Element != element {
			t.Fatalf("element %d: player %v, pet %v, enemy %v", element, stats(player), stats(pet), stats(b.Defenders[0]))
		}
	}
}

func TestEnemyExplicitOverridesPreserveOtherPlayerStats(t *testing.T) {
	b := New(nil, []Enemy{{Level: 10, Element: game.Fire, HP: 12345, SP: 17, Attack: 678}})
	f := b.Defenders[0]
	if f.MaxHP != 12345 || f.HP != 12345 || f.Atk != 678 || f.Matk != 678 || f.Def != 20 || f.Mdef != 20 || f.Spd != 16 || f.MaxSP != 17 || f.SP != 17 {
		t.Fatalf("scripted enemy overrides: %+v", f)
	}
}

func TestEnemyMissingVitalsDoNotUsePlayerGrowth(t *testing.T) {
	for _, level := range []int{1, 100} {
		b := New(nil, []Enemy{{Level: level, Element: game.Fire, Base: game.Attributes{Constitution: 100, Wisdom: 100}}})
		f := b.Defenders[0]
		if f.HP != 1 || f.MaxHP != 1 || f.SP != 0 || f.MaxSP != 0 {
			t.Fatalf("missing NPC vitals acquired player growth: %+v", f)
		}
	}
}

func TestEnemyReinforcementsUseStoredVitals(t *testing.T) {
	enemies := make([]Enemy, MaxEnemies+1)
	for i := range enemies {
		enemies[i] = Enemy{Template: 17000, Level: 1, HP: 36, SP: 100}
	}
	enemies[MaxEnemies] = Enemy{Template: 17002, Level: 20, HP: 40, SP: 0}
	b := New(nil, enemies)
	b.Defenders[0].HP = 0
	b.spawnReinforcements()
	f := b.Defenders[0]
	if f.Template != 17002 || f.HP != 40 || f.MaxHP != 40 || f.SP != 0 || f.MaxSP != 0 {
		t.Fatalf("reinforcement acquired formula vitals: %+v", f)
	}
}
