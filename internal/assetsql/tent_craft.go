package assetsql

import (
	_ "embed"
	"encoding/json"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// Source-authored furniture references; quest ladders are excluded. Seed only
// during import/migration so administrator changes survive later initialization.
//
//go:embed tent_floor_switches_defaults.json
var tentFloorSwitchDefaults []byte

func tentFloorSwitchIDs() ([]uint16, error) {
	var ids []uint16
	err := json.Unmarshal(tentFloorSwitchDefaults, &ids)
	return ids, err
}

func initializeTentFloorSwitches(c *assets.Catalog) error {
	ids, err := tentFloorSwitchIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if item, ok := c.NativeItems[id]; ok {
			item.Definition.TentFloorSwitch = true
			c.NativeItems[id] = item
			c.Items[id] = item.Definition
		}
	}
	return nil
}

func migrateTentCraft(tx *gorm.DB) error {
	if err := addTentCraftColumns(tx); err != nil {
		return err
	}
	// Existing recipe values become their reset baseline without changing any
	// administrator duration. Future edits modify only ValueDurationSeconds.
	if tx.Migrator().HasTable(&ManufacturingRow{}) {
		if err := tx.Model(&ManufacturingRow{}).Where("1 = 1").Update("value_default_duration_seconds", gorm.Expr("value_duration_seconds")).Error; err != nil {
			return err
		}
	}
	ids, err := tentFloorSwitchIDs()
	if err != nil {
		return err
	}
	if tx.Migrator().HasTable(&NativeItemsRow{}) {
		return tx.Model(&NativeItemsRow{}).Where("native_items_key IN ?", ids).Update("value_definition_tent_floor_switch", true).Error
	}
	return nil
}

func addTentCraftColumns(tx *gorm.DB) error {
	for _, column := range []struct {
		model any
		name  string
	}{
		{&ManufacturingRow{}, "ValueDefaultDurationSeconds"},
		{&NativeItemsRow{}, "ValueDefinitionTentFloorSwitch"},
	} {
		if tx.Migrator().HasTable(column.model) && !tx.Migrator().HasColumn(column.model, column.name) {
			if err := tx.Migrator().AddColumn(column.model, column.name); err != nil {
				return err
			}
		}
	}
	return nil
}
