package assets

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestCollectionSpotResolutionAndValidation(t *testing.T) {
	spot := CollectionSpot{Location: "Test", Source: "owner", Kind: CollectionKindVacuum, MapID: 1, MapCandidates: []uint16{1}, PositionKnown: true, X: 100, Y: 200, Rewards: []CollectionSpotReward{{ItemID: 42, SourceItemID: 99}}}
	items := map[uint16]game.ItemDefinition{42: {ID: 42}}
	maps := map[uint16]Map{1: {ID: 1}}
	if !spot.Resolved() || ValidateCollectionSpots(map[uint16]CollectionSpot{1: spot}, items, maps) != nil {
		t.Fatal(spot)
	}
	owner := spot
	owner.Rewards = []CollectionSpotReward{{ItemID: 42}}
	if err := ValidateCollectionSpots(map[uint16]CollectionSpot{1: owner}, items, maps); err != nil {
		t.Fatal("owner reward needs no website ID", err)
	}
	for _, change := range []func(*CollectionSpot){func(s *CollectionSpot) { s.MapID = 0 }, func(s *CollectionSpot) { s.PositionKnown = false; s.X = 0; s.Y = 0 }, func(s *CollectionSpot) { s.Rewards = nil }, func(s *CollectionSpot) { s.Rewards = []CollectionSpotReward{{SourceItemID: 99}} }} {
		next := spot
		change(&next)
		if next.Resolved() {
			t.Fatal("incomplete spot resolved", next)
		}
		if err := ValidateCollectionSpots(map[uint16]CollectionSpot{1: next}, items, maps); err != nil {
			t.Fatal("partial definition lost", err)
		}
	}
	spot.MapCandidates = []uint16{2}
	if ValidateCollectionSpots(map[uint16]CollectionSpot{1: spot}, items, maps) == nil {
		t.Fatal("unknown map accepted")
	}
}
