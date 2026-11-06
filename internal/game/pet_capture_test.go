package game

import "testing"

func TestCapturedPetInitialAttributeBudget(t *testing.T) {
	for _, stats := range []Attributes{{Strength: 11, Constitution: 13, Intelligence: 5, Wisdom: 7, Agility: 9}, {Strength: 10, Constitution: 12, Intelligence: 4, Wisdom: 6, Agility: 8}, {Strength: 1}} {
		template := PetTemplate{Stats: stats}
		p := NewCapturedPet(17100, "Pet", 1, template, nil, func(n int) int { return n - 1 })
		total := func(a Attributes) int {
			return int(a.Strength) + int(a.Constitution) + int(a.Intelligence) + int(a.Wisdom) + int(a.Agility)
		}
		if total(p.Base) != total(stats)/2 || p.Level != 1 || p.Amity != 60 || p.HP != p.MaxHP || p.SP != p.MaxSP {
			t.Fatal("initial captured stats", stats, p)
		}
		story := NewPet(14162, "Roca", 1, template, nil)
		if story.Base != (Attributes{stats.Strength / 2, stats.Constitution / 2, stats.Intelligence / 2, stats.Wisdom / 2, stats.Agility / 2}) {
			t.Fatal("unspecified recruitment attributes did not use half template")
		}
	}
}
