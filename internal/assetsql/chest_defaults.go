package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strings"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// Authored new categories from ChestDropManager at 1ff5bba; old pool edits win.
//
//go:embed chest_defaults_v14.json
var chestDefaultsV14 []byte

func seedNewChestDefaults(c *assets.Catalog) error {
	var defaults []assets.ChestPool
	if err := json.Unmarshal(chestDefaultsV14, &defaults); err != nil {
		return err
	}
	for _, pool := range defaults {
		exists := false
		for _, old := range c.ChestPools {
			if old.MapID == 0 && strings.EqualFold(old.Category, pool.Category) {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		complete := true
		for _, r := range pool.Rewards {
			if r.Gold == 0 {
				if _, ok := c.Items[r.Item]; !ok {
					complete = false
				}
			}
		}
		if complete {
			c.ChestPools = append(c.ChestPools, pool)
		}
	}
	return nil
}

func appendMissingChestDefaults(tx *gorm.DB) error {
	old, err := readChestPools(tx)
	if err != nil {
		return err
	}
	native, err := readNativeItems(tx)
	if err != nil {
		return err
	}
	c := &assets.Catalog{ChestPools: old, Items: map[uint16]game.ItemDefinition{}}
	for id, n := range native {
		c.Items[id] = n.Definition
	}
	if err := seedNewChestDefaults(c); err != nil {
		return err
	}
	for i := len(old); i < len(c.ChestPools); i++ {
		p := c.ChestPools[i]
		row := ChestPoolsRow{ChestPoolsOrdinal: i, ValueMapID: p.MapID, ValueCategory: p.Category, ValueRespawnSeconds: p.RespawnSeconds, ValueRewardsPresent: true}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := writeChestPoolsRewards(tx, p.Rewards, i); err != nil {
			return err
		}
	}
	if len(c.ChestPools) > len(old) {
		return tx.Model(&catalogPresence{}).Where("id = ?", catalogMetadataID).Update("chest_pools", true).Error
	}
	return nil
}

//go:embed chest_prop_categories_defaults.json
var chestPropCategoriesDefaults []byte

func defaultChestPropCategories() (map[uint16]string, error) {
	var values map[uint16]string
	if err := json.Unmarshal(chestPropCategoriesDefaults, &values); err != nil {
		return values, err
	}
	return values, validateChestPropCategories(values)
}

func validateChestPropCategories(values map[uint16]string) error {
	for id, kind := range values {
		if id == 0 || kind != "crate" && kind != "chest" {
			return fmt.Errorf("invalid chest prop category")
		}
	}
	return nil
}

func migrateChestDefaults(tx *gorm.DB) error {
	if !tx.Migrator().HasColumn(&ChestPoolsRewardsRow{}, "ValueGold") {
		if err := tx.Migrator().AddColumn(&ChestPoolsRewardsRow{}, "ValueGold"); err != nil {
			return err
		}
	}
	if err := migrateCatalogDataset(tx, "catalog_chest_prop_categories", "chest_prop_categories", func() error {
		values, err := defaultChestPropCategories()
		if err != nil {
			return err
		}
		return writeChestPropCategories(tx, values)
	}); err != nil {
		return err
	}
	return appendMissingChestDefaults(tx)
}
