package inventory

import (
	"testing"
	"wonderland-gonline/client/wlo/world"
)

func TestPetInventoryUsesCatalogElementAndPlayerFormula(t *testing.T) {
	s := State{PetElement: func(id uint16) byte {
		if id != 17411 {
			t.Fatalf("unexpected template %d", id)
		}
		return 4
	}}
	p := UsePet{ID: 17411, Stats: world.Stats{Level: 4, STR: 2, CON: 2, INT: 1, WIS: 1, AGI: 2}}
	s.recomputePet(&p)
	if p.Stats.Element != 4 || p.Stats.Combat != [5]uint16{10, 12, 8, 10, 12} || p.Stats.MaxHP != 194 || p.Stats.MaxSP != 105 {
		t.Fatalf("Floating inventory stats: %+v", p.Stats)
	}
}
