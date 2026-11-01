package assetsql

import (
	_ "embed"
	"encoding/json"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

//go:embed arcade_defaults.json
var arcadeDefaults []byte

// The seed is used only during explicit creation/migration. Existing SQL edits
// are never replaced on restart or when migration is repeated.
func defaultArcades(items map[uint16]game.ItemDefinition) ([]assets.ArcadeGame, error) {
	var result []assets.ArcadeGame
	if err := json.Unmarshal(arcadeDefaults, &result); err != nil {
		return nil, err
	}
	for i, g := range result {
		for _, r := range g.Rewards {
			if _, ok := items[r.ItemID]; !ok {
				result[i].Enabled = false
			}
		}
	}
	return result, assets.ValidateArcades(result, items)
}

// appendMissingArcades writes the seed's machines whose kinds the database
// lacks after its existing rows; existing machines are left untouched.
func appendMissingArcades(tx *gorm.DB) error {
	existing, err := readArcades(tx)
	if err != nil {
		return err
	}
	have := map[byte]bool{}
	for _, g := range existing {
		have[g.Kind] = true
	}
	nativeItems, err := readNativeItems(tx)
	if err != nil {
		return err
	}
	items := map[uint16]game.ItemDefinition{}
	for id, item := range nativeItems {
		items[id] = item.Definition
	}
	defaults, err := defaultArcades(items)
	if err != nil {
		return err
	}
	var next int
	if err := tx.Model(&ArcadesRow{}).Select("COALESCE(MAX(arcades_ordinal) + 1, 0)").Scan(&next).Error; err != nil {
		return err
	}
	var missing []assets.ArcadeGame
	for _, g := range defaults {
		if !have[g.Kind] {
			missing = append(missing, g)
		}
	}
	if err := assets.ValidateArcades(append(existing, missing...), items); err != nil {
		return err
	}
	for _, g := range missing {
		row := ArcadesRow{ArcadesOrdinal: next, ValueKind: g.Kind, ValueEnabled: g.Enabled, ValuePointCost: g.PointCost,
			ValueTokenCount: g.TokenCount, ValueCooldownMilliseconds: g.CooldownMilliseconds, ValueRewardsPresent: g.Rewards != nil}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := writeArcadesRewards(tx, g.Rewards, next); err != nil {
			return err
		}
		next++
	}
	return nil
}
