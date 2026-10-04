package game

import "testing"

func TestPetAllocationBudgetAndOverflow(t *testing.T) {
	p := Pet{StatPoints: 3, Base: Attributes{Strength: 65535, Intelligence: 2}}
	if !p.Allocate([]StatAllocation{{StatSTR, 1}, {StatINT, 3}, {StatCON, 2}}) {
		t.Fatal("no allocation")
	}
	if p.StatPoints != 0 || p.Base.Strength != 65535 || p.Base.Intelligence != 5 || p.Base.Constitution != 0 {
		t.Fatal(p)
	}
	before := p.Base
	if p.Allocate([]StatAllocation{{StatAGI, 1}}) || p.Base != before {
		t.Fatal("overspent points")
	}
}
