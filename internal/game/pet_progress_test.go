package game

import "testing"

func TestPetAllocationBudgetAndWrap(t *testing.T) {
	p := Pet{StatPoints: 3, Base: Attributes{Strength: 65535, Intelligence: 2}}
	if !p.Allocate([]StatAllocation{{StatSTR, 1}, {StatINT, 3}, {StatCON, 2}}) {
		t.Fatal("no allocation")
	}
	if p.StatPoints != 0 || p.Base.Strength != 0 || p.Base.Intelligence != 2 || p.Base.Constitution != 2 {
		t.Fatal(p)
	}
	before := p.Base
	if p.Allocate([]StatAllocation{{StatAGI, 1}}) || p.Base != before {
		t.Fatal("overspent points")
	}
}
