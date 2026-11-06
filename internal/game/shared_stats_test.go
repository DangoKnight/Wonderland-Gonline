package game

import "testing"

func TestSharedElementalStatsGolden(t *testing.T) {
	attributes := Attributes{Strength: 5, Constitution: 5, Intelligence: 7, Wisdom: 9, Agility: 11}
	for _, tc := range []struct {
		element byte
		want    Combat
	}{
		{Earth, Combat{MaxHP: 222, MaxSP: 179, ATK: 24, DEF: 35, MAT: 28, MDF: 42, SPD: 36}},
		{Water, Combat{MaxHP: 222, MaxSP: 179, ATK: 24, DEF: 29, MAT: 28, MDF: 40, SPD: 36}},
		{Fire, Combat{MaxHP: 222, MaxSP: 179, ATK: 30, DEF: 29, MAT: 30, MDF: 40, SPD: 36}},
		{Wind, Combat{MaxHP: 222, MaxSP: 179, ATK: 24, DEF: 29, MAT: 28, MDF: 40, SPD: 41}},
	} {
		c := Character{Level: 10, Element: tc.element, Base: attributes}
		p := Pet{ID: 17411, Level: 10, Base: attributes}
		template := PetTemplate{Element: tc.element}
		if got := c.Combat(nil); got != tc.want {
			t.Fatalf("player element %d: %+v, want %+v", tc.element, got, tc.want)
		}
		if got := p.Combat(nil, template); got != tc.want {
			t.Fatalf("pet element %d: %+v, want %+v", tc.element, got, tc.want)
		}
		p.Potential, c.Potential = 5, 5
		if p.Combat(nil, template) != c.Combat(nil) {
			t.Fatalf("potential diverged for element %d", tc.element)
		}
	}
}

func TestFloatingDefenseUsesWindFormula(t *testing.T) {
	p := Pet{ID: 17411, Level: 4, Base: Attributes{Strength: 2, Constitution: 2, Intelligence: 1, Wisdom: 1, Agility: 2}}
	got := p.Combat(nil, PetTemplate{Element: Wind})
	want := Combat{MaxHP: 194, MaxSP: 105, ATK: 10, DEF: 12, MAT: 8, MDF: 10, SPD: 12}
	if got != want {
		t.Fatalf("Floating = %+v, want %+v", got, want)
	}
}
