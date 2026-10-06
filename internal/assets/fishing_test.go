package assets

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestFishingWeightBoundariesAndShore(t *testing.T) {
	f := FishingRules{Rewards: []FishingReward{{ItemID: 1, Grade: 1, Weight: 3}, {ItemID: 2, Grade: 7, Weight: 1}}, SkillBonusPercent: 5}
	rod := FishingRod{MaxGrade: 6}
	if r, ok := f.Select(rod, 0, 299); !ok || r.ItemID != 1 {
		t.Fatal(r, ok)
	}
	if _, ok := f.Select(rod, 0, 300); ok {
		t.Fatal("out of range roll accepted")
	}
	rod.MaxGrade = 8
	if r, ok := f.Select(rod, 0, 300); !ok || r.ItemID != 2 {
		t.Fatal(r, ok)
	}
	if f.Weight(f.Rewards[1], rod, 10)*f.Weight(f.Rewards[0], rod, 0) <= f.Weight(f.Rewards[1], rod, 0)*f.Weight(f.Rewards[0], rod, 10) {
		t.Fatal("skill does not favor higher grades")
	}
	terrain := Terrain{Width: 40, Height: 40, GridWidth: 2, GridHeight: 2, Cells: []byte{0, 2, 0, 0}}
	if !terrain.FishingShore(0, 0) || terrain.FishingShore(0, 20) || terrain.FishingShore(40, 0) {
		t.Fatal("shore validation")
	}
	f.Enabled = true
	f.IntervalSeconds = 60
	f.Rods = []FishingRod{{ItemID: 3, MaxGrade: 8}}
	f.Maps = []uint16{1}
	items := map[uint16]game.ItemDefinition{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}}
	if err := ValidateFishing(f, items); err != nil {
		t.Fatal(err)
	}
	f.Rewards[1].Weight = 0
	if ValidateFishing(f, items) == nil {
		t.Fatal("zero reward weight accepted")
	}
}
