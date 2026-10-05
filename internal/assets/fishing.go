package assets

import (
	"fmt"
	"wonderland-go/internal/game"
)

const FishingWaterCell byte = 2
const fishingWaterRadiusCells = 7
const fishingMaximumWeight = 1000000
const fishingMaximumGrade = 10
const fishingMaximumIntervalSeconds = 86400
const fishingMaximumSkillBonusPercent = 100
const fishingMaximumRequirements = 254
const fishingWeightPercentScale = 100

type FishingRod struct {
	ItemID   uint16
	MaxGrade byte
}
type FishingReward struct {
	ItemID uint16
	Grade  byte
	Weight uint32
}
type FishingRules struct {
	Enabled           bool
	IntervalSeconds   uint32
	SkillBonusPercent uint32
	Skills            []uint16
	CatchRequirements []uint32
	Rods              []FishingRod
	Maps              []uint16
	Rewards           []FishingReward
}

func (f FishingRules) Rod(id uint16) (FishingRod, bool) {
	for _, r := range f.Rods {
		if r.ItemID == id {
			return r, true
		}
	}
	return FishingRod{}, false
}
func (f FishingRules) AllowsMap(id uint16) bool {
	for _, m := range f.Maps {
		if m == id {
			return true
		}
	}
	return false
}
func ValidateFishing(f FishingRules, items map[uint16]game.ItemDefinition) error {
	if !f.Enabled && f.IntervalSeconds == 0 && len(f.Rods) == 0 && len(f.Rewards) == 0 {
		return nil
	}
	if f.IntervalSeconds == 0 || f.IntervalSeconds > fishingMaximumIntervalSeconds || f.SkillBonusPercent > fishingMaximumSkillBonusPercent || len(f.Rods) == 0 || len(f.Rewards) == 0 || len(f.Maps) == 0 || len(f.CatchRequirements) > fishingMaximumRequirements {
		return fmt.Errorf("invalid fishing rules")
	}
	seen := map[uint16]bool{}
	for _, r := range f.Rods {
		if r.ItemID == 0 || r.MaxGrade == 0 || r.MaxGrade > fishingMaximumGrade || seen[r.ItemID] {
			return fmt.Errorf("invalid fishing rod")
		}
		seen[r.ItemID] = true
		if _, ok := items[r.ItemID]; !ok && f.Enabled {
			return fmt.Errorf("missing fishing rod %d", r.ItemID)
		}
	}
	seen = map[uint16]bool{}
	for _, r := range f.Rewards {
		if r.ItemID == 0 || r.Grade == 0 || r.Grade > fishingMaximumGrade || r.Weight == 0 || r.Weight > fishingMaximumWeight || seen[r.ItemID] {
			return fmt.Errorf("invalid fishing reward")
		}
		seen[r.ItemID] = true
		if _, ok := items[r.ItemID]; !ok && f.Enabled {
			return fmt.Errorf("missing fishing reward %d", r.ItemID)
		}
	}

	seen = map[uint16]bool{}
	for _, id := range f.Skills {
		if id == 0 || seen[id] {
			return fmt.Errorf("invalid fishing skill")
		}
		seen[id] = true
	}
	seen = map[uint16]bool{}
	for _, id := range f.Maps {
		if id == 0 || seen[id] {
			return fmt.Errorf("invalid fishing map")
		}
		seen[id] = true
	}
	for _, rod := range f.Rods {
		eligible := false
		for _, reward := range f.Rewards {
			if reward.Grade <= rod.MaxGrade {
				eligible = true
			}
		}
		if !eligible {
			return fmt.Errorf("fishing rod has no eligible rewards")
		}
	}
	for _, n := range f.CatchRequirements {
		if n == 0 {
			return fmt.Errorf("zero fishing proficiency requirement")
		}
	}
	return nil
}

// Higher skill grades increase the relative weight of higher-grade catches.
// Weighting is an authored policy: the native server probabilities are unavailable.
func (f FishingRules) Weight(r FishingReward, rod FishingRod, skill byte) uint64 {
	if r.Grade > rod.MaxGrade {
		return 0
	}
	return uint64(r.Weight) * (fishingWeightPercentScale + uint64(skill)*uint64(r.Grade)*uint64(f.SkillBonusPercent))
}
func (f FishingRules) Select(rod FishingRod, skill byte, roll uint64) (FishingReward, bool) {
	var total uint64
	for _, r := range f.Rewards {
		total += f.Weight(r, rod, skill)
	}
	if total == 0 || roll >= total {
		return FishingReward{}, false
	}
	for _, r := range f.Rewards {
		w := f.Weight(r, rod, skill)
		if roll < w {
			return r, true
		}
		roll -= w
	}
	return FishingReward{}, false
}

// Native FUN_0046a354 searches a 14x14 neighborhood for water; the
// legacy grid's one-cell border translates to offsets -7 through +6.
// FishingWater locates the same water cell used to validate casting. Its center
// provides a bounded source position for native catch presentation.
func (t Terrain) FishingWater(x, y uint16) (uint16, uint16, bool) {
	if !t.Walkable(int(x), int(y)) {
		return 0, 0, false
	}
	cx, cy := int(x)/TerrainCellSize, int(y)/TerrainCellSize
	for dx := -fishingWaterRadiusCells; dx < fishingWaterRadiusCells; dx++ {
		for dy := -fishingWaterRadiusCells; dy < fishingWaterRadiusCells; dy++ {
			gx, gy := cx+dx, cy+dy
			if gx >= 0 && gy >= 0 && gx < int(t.GridWidth) && gy < int(t.GridHeight) {
				at := gx*int(t.GridHeight) + gy
				wx, wy := gx*TerrainCellSize+TerrainCellSize/2, gy*TerrainCellSize+TerrainCellSize/2
				if at < len(t.Cells) && t.Cells[at] == FishingWaterCell && t.Contains(wx, wy) {
					return uint16(wx), uint16(wy), true
				}
			}
		}
	}
	return 0, 0, false
}
func (t Terrain) FishingShore(x, y uint16) bool {
	_, _, ok := t.FishingWater(x, y)
	return ok
}
