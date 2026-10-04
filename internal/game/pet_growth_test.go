package game

import (
	"math"
	"testing"
)

func TestPetGrowthAllWeightedIntervals(t *testing.T) {
	// Exhaust every integer outcome; exact counts verify the proportional odds
	// without flaky sampling and prove that the two smallest stats remain eligible.
	template := PetTemplate{Stats: Attributes{Strength: 10, Constitution: 20, Intelligence: 30, Wisdom: 40, Agility: 50}}
	for _, known := range []bool{true, false} {
		counts := [5]int{}
		for draw := 0; draw < 150; draw++ {
			p := Pet{Base: Attributes{Strength: 10, Constitution: 20, Intelligence: 30, Wisdom: 40, Agility: 50}}
			if known {
				p.Base = Attributes{Strength: 1, Constitution: 1, Intelligence: 1, Wisdom: 1, Agility: 1}
			}
			before := p.Base
			p.grow(template, known, func(total int) int {
				if total != 150 {
					t.Fatalf("total=%d, want 150", total)
				}
				return draw
			})
			changes := [5]int{int(p.Base.Strength) - int(before.Strength), int(p.Base.Constitution) - int(before.Constitution), int(p.Base.Intelligence) - int(before.Intelligence), int(p.Base.Wisdom) - int(before.Wisdom), int(p.Base.Agility) - int(before.Agility)}
			sum := 0
			for i, delta := range changes {
				sum += delta
				counts[i] += delta
				if delta < 0 || delta > 1 {
					t.Fatal("invalid stat change", changes)
				}
			}
			if sum != 1 {
				t.Fatal("growth did not allocate exactly one point", changes)
			}
		}
		if counts != [5]int{10, 20, 30, 40, 50} {
			t.Fatalf("known=%v: counts=%v", known, counts)
		}
	}
}

func TestPetGrowthZeroWeightsAndCaps(t *testing.T) {
	for draw := 0; draw < 5; draw++ {
		p := Pet{}
		p.grow(PetTemplate{}, true, func(total int) int {
			if total != 5 {
				t.Fatal(total)
			}
			return draw
		})
		values := [5]uint16{p.Base.Strength, p.Base.Constitution, p.Base.Intelligence, p.Base.Wisdom, p.Base.Agility}
		for i, v := range values {
			want := uint16(0)
			if i == draw {
				want = 1
			}
			if v != want {
				t.Fatal(values)
			}
		}
	}
	p := Pet{Base: Attributes{Strength: math.MaxUint16, Constitution: math.MaxUint16, Intelligence: 2, Wisdom: 3, Agility: 4}}
	template := PetTemplate{Stats: Attributes{Strength: 100, Constitution: 200, Intelligence: 3, Wisdom: 4, Agility: 5}}
	p.grow(template, true, func(total int) int {
		if total != 12 {
			t.Fatal("capped stats retained weight", total)
		}
		return 11
	})
	if p.Base != (Attributes{Strength: math.MaxUint16, Constitution: math.MaxUint16, Intelligence: 2, Wisdom: 3, Agility: 5}) {
		t.Fatal(p.Base)
	}
	p.Base = Attributes{Strength: math.MaxUint16, Constitution: math.MaxUint16, Intelligence: math.MaxUint16, Wisdom: math.MaxUint16, Agility: math.MaxUint16}
	before := p.Base
	p.grow(template, true, func(int) int { t.Fatal("rolled with all stats capped"); return 0 })
	if p.Base != before {
		t.Fatal("capped attributes changed", p.Base)
	}
}

func TestPetGrowthLevelsCanSelectLowestStat(t *testing.T) {
	template := PetTemplate{Stats: Attributes{Strength: 50, Constitution: 40, Intelligence: 30, Wisdom: 20, Agility: 1}}
	p := NewPet(17100, "Pet", 1, template, nil)
	calls := 0
	gained := p.GainExp(49, template, true, func(total int) int {
		calls++
		if total != 141 {
			t.Fatal(total)
		}
		return total - 1
	})
	if gained != 2 || calls != 2 || p.Level != 3 || p.Exp != 0 || p.Base != (Attributes{Strength: 50, Constitution: 40, Intelligence: 30, Wisdom: 20, Agility: 3}) {
		t.Fatal("lowest stat did not grow across levels", p, gained, calls)
	}
}
