package assets

import (
	"fmt"
	"slices"
	"wonderland-gonline/internal/game"
)

const CollectionKindUnresolved = "unresolved"
const CollectionKindVacuum = "vacuum"

// CollectionSpot is a reference-backed definition. Zero IDs and an unknown
// position preserve unresolved references; they must not be treated as live nodes.
// Timing, tool restrictions and probabilities are not specified by this source.
type CollectionSpot struct {
	Location      string                 `json:"location"`
	Region        string                 `json:"region"`
	Source        string                 `json:"source"`
	Kind          string                 `json:"kind"`
	MapID         uint16                 `json:"map_id"`
	MapCandidates []uint16               `json:"map_candidates"`
	PositionKnown bool                   `json:"position_known"`
	X             uint32                 `json:"x"`
	Y             uint32                 `json:"y"`
	Rewards       []CollectionSpotReward `json:"rewards"`
}

type CollectionSpotReward struct {
	ItemID       uint16 `json:"item_id"`
	SourceItemID uint32 `json:"source_item_id"`
}

// Resolved describes location/reward data completeness, not gameplay support.
func (s CollectionSpot) Resolved() bool {
	if s.MapID == 0 || !s.PositionKnown || len(s.Rewards) == 0 {
		return false
	}
	for _, r := range s.Rewards {
		if r.ItemID == 0 {
			return false
		}
	}
	return true
}

func ValidateCollectionSpots(spots map[uint16]CollectionSpot, items map[uint16]game.ItemDefinition, maps map[uint16]Map) error {
	for id, spot := range spots {
		if id == 0 || spot.Location == "" || spot.Source == "" || spot.Kind != CollectionKindVacuum && spot.Kind != CollectionKindUnresolved {
			return fmt.Errorf("invalid collection spot %d", id)
		}
		if !spot.PositionKnown && (spot.X != 0 || spot.Y != 0) {
			return fmt.Errorf("collection spot %d has unknown position with coordinates", id)
		}
		seen := map[uint16]bool{}
		for _, mid := range spot.MapCandidates {
			if _, ok := maps[mid]; !ok || mid == 0 || seen[mid] {
				return fmt.Errorf("invalid collection map candidate %d/%d", id, mid)
			}
			seen[mid] = true
		}
		if spot.MapID != 0 && !slices.Contains(spot.MapCandidates, spot.MapID) {
			return fmt.Errorf("collection map not a candidate %d", id)
		}
		rewards := map[uint32]bool{}
		itemsSeen := map[uint16]bool{}
		for _, r := range spot.Rewards {
			if r.SourceItemID == 0 && r.ItemID == 0 || r.SourceItemID != 0 && rewards[r.SourceItemID] || r.ItemID != 0 && itemsSeen[r.ItemID] {
				return fmt.Errorf("invalid collection reward %d", id)
			}
			if r.SourceItemID != 0 {
				rewards[r.SourceItemID] = true
			}
			if r.ItemID != 0 {
				itemsSeen[r.ItemID] = true
			}
			if r.ItemID != 0 {
				if _, ok := items[r.ItemID]; !ok {
					return fmt.Errorf("unknown collection item %d/%d", id, r.ItemID)
				}
			}
		}
	}
	return nil
}
