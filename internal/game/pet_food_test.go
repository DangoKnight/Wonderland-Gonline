package game

import (
	"errors"
	"math"
	"testing"
)

func TestPetFoodAttributeSelectionAndLifetimeCap(t *testing.T) {
	for index := 0; index < 5; index++ {
		p := Pet{Base: Attributes{1, 2, 3, 4, 5}, StatFoodUses: 1199}
		ok, err := p.AddFoodStat(PetTemplate{Stats: Attributes{1, 1, 1, 1, 1}}, true, func(n int) (int, error) {
			if n != 5 {
				t.Fatal(n)
			}
			return index, nil
		})
		got := []uint16{p.Base.Strength, p.Base.Constitution, p.Base.Intelligence, p.Base.Wisdom, p.Base.Agility}
		if err != nil || !ok || p.StatFoodUses != 1200 || got[index] != uint16(index+2) {
			t.Fatal(p, err)
		}
		before := p
		if ok, err := p.AddFoodStat(PetTemplate{}, true, func(int) (int, error) { t.Fatal("capped pet rolled"); return 0, nil }); ok || err != nil || p.Base != before.Base || p.StatFoodUses != before.StatFoodUses {
			t.Fatal("cap bypassed")
		}
	}
	p := Pet{Base: Attributes{Strength: math.MaxUint16}}
	if ok, err := p.AddFoodStat(PetTemplate{}, true, func(n int) (int, error) {
		if n != 4 {
			t.Fatal(n)
		}
		return 0, nil
	}); err != nil || !ok || p.Base.Strength != math.MaxUint16 || p.Base.Constitution != 1 {
		t.Fatal("attribute overflow", p, err)
	}
}

func TestPetFoodGrowthWeightedIntervals(t *testing.T) {
	equipment, items := petGrowthEquipment()
	for _, tc := range []struct {
		name    string
		formula PetGrowthFormula
		known   bool
		weights [5]int
	}{
		{"template", PetGrowthBaseStats, true, [5]int{10, 20, 30, 40, 50}},
		{"unknown template", PetGrowthBaseStats, false, [5]int{5, 5, 5, 5, 5}},
		{"combat", PetGrowthCombatStats, true, [5]int{44, 25, 54, 35, 67}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			total := 0
			for _, weight := range tc.weights {
				total += weight
			}
			counts := [5]int{}
			for draw := 0; draw < total; draw++ {
				p := Pet{ID: 17100, Level: 10, Base: Attributes{5, 5, 5, 5, 5}, Equipment: equipment}
				ok, err := p.AddFoodStat(PetTemplate{Stats: Attributes{10, 20, 30, 40, 50}}, tc.known, func(n int) (int, error) {
					if n != total {
						t.Fatalf("weights %d != %d", n, total)
					}
					return draw, nil
				}, PetGrowthOptions{Formula: tc.formula, Items: items})
				if !ok || err != nil || p.StatFoodUses != 1 || p.Level != 10 || p.Exp != 0 {
					t.Fatal(p, err)
				}
				for i, value := range []uint16{p.Base.Strength, p.Base.Constitution, p.Base.Intelligence, p.Base.Wisdom, p.Base.Agility} {
					counts[i] += int(value) - 5
				}
			}
			if counts != tc.weights {
				t.Fatal(counts, tc.weights)
			}
		})
	}
}

func TestPetFoodGrowthRollErrorsAndAllCapped(t *testing.T) {
	for _, draw := range []int{-1, 5} {
		p := Pet{}
		ok, err := p.AddFoodStat(PetTemplate{}, false, func(int) (int, error) { return draw, nil })
		if ok || !errors.Is(err, ErrPetGrowthRoll) || p.Base != (Attributes{}) || p.StatFoodUses != 0 {
			t.Fatal(p, err)
		}
	}
	p := Pet{Base: Attributes{math.MaxUint16, math.MaxUint16, math.MaxUint16, math.MaxUint16, math.MaxUint16}}
	if ok, err := p.AddFoodStat(PetTemplate{}, true, func(int) (int, error) { t.Fatal("all capped pet rolled"); return 0, nil }); ok || err != nil || p.StatFoodUses != 0 {
		t.Fatal(p, err)
	}
}
