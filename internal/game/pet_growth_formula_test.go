package game

import "testing"

func petGrowthEquipment() (Equipment, map[uint16]ItemDefinition) {
	var equipment Equipment
	equipment[0] = Item{ID: 1, Count: 1}
	equipment[1] = Item{ID: 2, Count: 1}
	equipment[2] = Item{ID: 3, Count: 1}
	items := map[uint16]ItemDefinition{
		1: {ID: 1, Status: [2]uint16{210, 211}, Values: [2]int32{120, 95}},
		2: {ID: 2, Status: [2]uint16{215, 216}, Values: [2]int32{130, 105}},
		3: {ID: 3, Status: [2]uint16{214, 0}, Values: [2]int32{140, 0}},
	}
	return equipment, items
}

func TestPetGrowthCombatStatsWeightedIntervals(t *testing.T) {
	equipment, items := petGrowthEquipment()
	counts := [5]int{}
	for draw := 0; draw < 285; draw++ {
		p := Pet{ID: 17100, Level: 10, Base: Attributes{5, 5, 5, 5, 5}, Equipment: equipment}
		p.grow(PetTemplate{Stats: Attributes{Strength: 1000}}, true, func(total int) int {
			if total != 285 {
				t.Fatalf("total=%d, want 285", total)
			}
			return draw
		}, PetGrowthOptions{Formula: PetGrowthCombatStats, Items: items})
		deltas := [5]int{int(p.Base.Strength) - 5, int(p.Base.Constitution) - 5, int(p.Base.Intelligence) - 5, int(p.Base.Wisdom) - 5, int(p.Base.Agility) - 5}
		sum := 0
		for i, delta := range deltas {
			sum += delta
			counts[i] += delta
		}
		if sum != 1 {
			t.Fatal("incorrect point count", deltas)
		}
	}
	// Independent expected combat weights include positive and negative equipment.
	if counts != [5]int{44, 84, 54, 36, 67} {
		t.Fatal(counts)
	}
}

func TestPetGrowthCombatElementAndRecalculation(t *testing.T) {
	equipment, items := petGrowthEquipment()
	p := Pet{ID: 12178, Level: 10, Base: Attributes{5, 5, 5, 5, 5}, Equipment: equipment}
	weights := p.growthWeights(PetTemplate{}, false, []PetGrowthOptions{{Formula: PetGrowthCombatStats, Items: items}})
	want := [5]int{44, 24, 54, 36, 67} // Existing Earth companion combat rules reduce the DEF level term.
	for i, v := range weights {
		if v != want[i] {
			t.Fatal(weights)
		}
	}
	p = Pet{ID: 17100, Level: 1, Base: Attributes{5, 5, 5, 5, 5}}
	calls := 0
	levels := p.GainExp(49, PetTemplate{}, false, func(total int) int {
		// Level 2 weights: 13+25+13+15+14; AGI grows, then level 3 weights:
		// 14+33+14+17+18. Each draw sees the new level and preceding point.
		wantTotal := []int{80, 96}[calls]
		calls++
		if total != wantTotal {
			t.Fatalf("draw %d total=%d, want %d", calls, total, wantTotal)
		}
		return total - 1
	}, PetGrowthOptions{Formula: PetGrowthCombatStats})
	if levels != 2 || calls != 2 || p.Base != (Attributes{5, 5, 5, 5, 7}) {
		t.Fatal(p, levels, calls)
	}
}

func TestPetGrowthCombatCapsAndMinimumWeights(t *testing.T) {
	p := Pet{Level: 0}
	p.grow(PetTemplate{}, false, func(total int) int {
		if total != 5 {
			t.Fatal(total)
		}
		return 4
	}, PetGrowthOptions{Formula: PetGrowthCombatStats})
	if p.Base.Agility != 1 {
		t.Fatal(p.Base)
	}
	p = Pet{Level: 10, Base: Attributes{Strength: 65535, Constitution: 5, Intelligence: 5, Wisdom: 5, Agility: 5}}
	p.grow(PetTemplate{}, false, func(total int) int {
		if total != 171 {
			t.Fatal("capped stat retained", total)
		}
		return 0
	}, PetGrowthOptions{Formula: PetGrowthCombatStats})
	if p.Base.Strength != 65535 || p.Base.Constitution != 6 {
		t.Fatal(p.Base)
	}
	for _, f := range []PetGrowthFormula{PetGrowthBaseStats, PetGrowthCombatStats} {
		if !f.Valid() {
			t.Fatal(f)
		}
	}
	if PetGrowthFormula("unknown").Valid() {
		t.Fatal("unknown formula accepted")
	}
}
