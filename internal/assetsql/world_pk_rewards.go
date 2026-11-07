package assetsql

import (
	_ "embed"
	"encoding/json"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed world_pk_rewards_defaults.json
var worldPKRewardDefaults []byte

func defaultWorldPKRewards() (assets.WorldPKRewardDefinitions, error) {
	var values assets.WorldPKRewardDefinitions
	if err := json.Unmarshal(worldPKRewardDefaults, &values); err != nil {
		return values, err
	}
	return values, assets.ValidateWorldPKRewards(values)
}

func migrateWorldPKRewards(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_world_p_k_rewards", "", func() error {
		values, err := defaultWorldPKRewards()
		if err != nil {
			return err
		}
		return writeWorldPKRewards(tx, values)
	})
}
