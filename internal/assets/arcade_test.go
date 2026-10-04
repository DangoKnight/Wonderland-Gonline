package assets

import (
	"math"
	"testing"
	"wonderland-go/internal/game"
)

func TestArcadeWeightedSelectionAndValidation(t *testing.T) {
	g := ArcadeGame{Kind: 6, Enabled: true, PointCost: 20, CooldownMilliseconds: 3000, Rewards: []ArcadeReward{
		{Index: 1, ItemID: 32162, Quantity: 1, Weight: 3},
		{Index: 2, ItemID: 46004, Quantity: 1, Weight: 0},
		{Index: 3, ItemID: 33044, Quantity: 1, Weight: 1},
	}}
	items := map[uint16]game.ItemDefinition{32162: {ID: 32162}, 46004: {ID: 46004}, 33044: {ID: 33044}}
	if err := ValidateArcades([]ArcadeGame{g}, items); err != nil {
		t.Fatal(err)
	}
	for roll, want := range []byte{1, 1, 1, 3} {
		r, ok := g.RewardForRoll(int64(roll))
		if !ok || r.Index != want {
			t.Fatal(roll, r, ok)
		}
	}
	for _, roll := range []int64{-1, 4} {
		if _, ok := g.RewardForRoll(roll); ok {
			t.Fatal("invalid roll accepted")
		}
	}
	g.Rewards[0].Weight = math.MaxInt64
	if err := ValidateArcades([]ArcadeGame{g}, items); err == nil {
		t.Fatal("weight overflow accepted")
	}
	g.Rewards[0].Weight = 0
	g.Rewards[2].Weight = 0
	if err := ValidateArcades([]ArcadeGame{g}, items); err == nil {
		t.Fatal("enabled empty pool accepted")
	}
}
