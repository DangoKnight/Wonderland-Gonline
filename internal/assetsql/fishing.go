package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

//go:embed fishing_defaults.json
var fishingDefaults []byte

func defaultFishing(items map[uint16]game.ItemDefinition) (assets.FishingRules, error) {
	var f assets.FishingRules
	if err := json.Unmarshal(fishingDefaults, &f); err != nil {
		return f, err
	}
	for _, r := range f.Rods {
		if _, ok := items[r.ItemID]; !ok {
			f.Enabled = false
		}
	}
	for _, r := range f.Rewards {
		if _, ok := items[r.ItemID]; !ok {
			f.Enabled = false
		}
	}
	return f, assets.ValidateFishing(f, items)
}

//go:embed fishing_v7_weights.json
var fishingV7Weights []byte

// Upgrade only unchanged v7 fish/seafood defaults. Edited weights/grades and
// removed rewards remain authoritative. The caller owns the migration transaction.
func migrateFishingWeights(tx *gorm.DB) error {
	var previous []assets.FishingReward
	var defaults assets.FishingRules
	if err := json.Unmarshal(fishingV7Weights, &previous); err != nil {
		return err
	}
	if err := json.Unmarshal(fishingDefaults, &defaults); err != nil {
		return err
	}
	current := make(map[uint16]assets.FishingReward, len(defaults.Rewards))
	for _, reward := range defaults.Rewards {
		current[reward.ItemID] = reward
	}
	for _, old := range previous {
		next, ok := current[old.ItemID]
		if !ok || next.Grade != old.Grade {
			return fmt.Errorf("missing fishing weight upgrade for %d", old.ItemID)
		}
		if err := tx.Model(&FishingRewardsRow{}).Where(map[string]any{"fishing_key": catalogMetadataID, "value_item_id": old.ItemID, "value_grade": old.Grade, "value_weight": old.Weight}).Update("value_weight", next.Weight).Error; err != nil {
			return err
		}
	}
	return nil
}
