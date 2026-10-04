package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

// Source: MarriageManager, TentManufactureManager, GatheringManager and AlchemyManager.
// This embedded seed is consumed by explicit offline schema migration only.
//
//go:embed economy_defaults.json
var economyDefaults []byte

func defaultEconomy() (assets.Economy, error) {
	var result assets.Economy
	err := json.Unmarshal(economyDefaults, &result)
	return result, err
}

const economyPercentScale = 100
const maxGatheringIntervalSeconds = uint32(24 * time.Hour / time.Second)

func validateEconomy(e assets.Economy) error {
	validPercent := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= economyPercentScale }
	if !validPercent(e.Synthesis.DefaultSuccessPercent) {
		return fmt.Errorf("invalid synthesis default chance")
	}
	rates := map[[3]uint16]bool{}
	for _, r := range e.Synthesis.Rates {
		key := [3]uint16{min(r.Input1, r.Input2), max(r.Input1, r.Input2), r.Output}
		if r.Input1 == 0 || r.Input2 == 0 || r.Output == 0 || r.Fee > game.MaxGold || !validPercent(r.SuccessPercent) || rates[key] {
			return fmt.Errorf("invalid or duplicate synthesis rate")
		}
		rates[key] = true
	}

	if e.Marriage.Fee > game.MaxGold {
		return fmt.Errorf("marriage fee exceeds gold limit")
	}
	seen := map[string]bool{}
	for _, r := range e.Manufacturing {
		if r.Fee > game.MaxGold || (r.SuccessPercent != nil && !validPercent(*r.SuccessPercent)) {
			return fmt.Errorf("invalid manufacturing chance or fee")
		}
		if strings.TrimSpace(r.Workbench) == "" || r.Output.ItemID == 0 || r.Output.Count == 0 || r.Inputs[0].ItemID == 0 || r.Inputs[0].Count == 0 {
			return fmt.Errorf("invalid manufacturing recipe")
		}
		for _, in := range r.Inputs {
			if (in.ItemID == 0) != (in.Count == 0) {
				return fmt.Errorf("invalid manufacturing input")
			}
		}
		key := fmt.Sprintf("%s:%d:%d:%d:%d", strings.ToLower(r.Workbench), r.Inputs[0].ItemID, r.Inputs[0].Count, r.Inputs[1].ItemID, r.Inputs[1].Count)
		if seen[key] {
			return fmt.Errorf("duplicate manufacturing recipe")
		}
		seen[key] = true
	}
	kinds := map[byte]bool{}
	for _, pool := range e.Gathering {
		if pool.Kind < assets.GatheringFishing || pool.Kind > assets.GatheringWoodcutting || kinds[pool.Kind] || pool.IntervalSeconds == 0 || pool.IntervalSeconds > maxGatheringIntervalSeconds || len(pool.Items) == 0 {
			return fmt.Errorf("invalid gathering pool")
		}
		kinds[pool.Kind] = true
		for _, id := range pool.Items {
			if id == 0 {
				return fmt.Errorf("invalid gathering item")
			}
		}
	}
	return nil
}
