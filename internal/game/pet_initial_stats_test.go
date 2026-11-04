package game

import "testing"

func TestRecruitedPetInitialRangesRespectDesiredTotal(t *testing.T) {
	template := PetTemplate{Stats: Attributes{15, 20, 5, 8, 12}, InitialStats: &PetInitialStats{Minimum: Attributes{7, 10, 2, 4, 6}, Maximum: Attributes{7, 11, 2, 5, 7}, TotalPoints: 30}}
	for choice := 0; choice < 3; choice++ {
		calls := 0
		p := NewPet(12178, "Robinson", 1, template, nil, func(n int) int {
			if n != 3 {
				t.Fatal(n)
			}
			calls++
			return choice
		})
		want := []Attributes{{7, 11, 2, 4, 6}, {7, 10, 2, 5, 6}, {7, 10, 2, 4, 7}}[choice]
		if p.Base != want || calls != 1 || p.Level != 1 || p.Potential != 0 || p.HP != p.MaxHP || p.SP != p.MaxSP {
			t.Fatal("incorrect fresh recruit", p, calls, want)
		}
	}
	// Even when a monster template has a recruitment override, wild capture
	// retains its separate half-total rounding rule and caller-supplied RNG.
	captured := NewCapturedPet(12178, "Pet", 1, template, nil, func(n int) int { return n - 1 })
	if captured.Base.Strength != 7 || captured.Base.Intelligence != 2 || captured.Base.Strength+captured.Base.Constitution+captured.Base.Intelligence+captured.Base.Wisdom+captured.Base.Agility != 30 {
		t.Fatal("capture reused recruitment override", captured.Base)
	}
}
