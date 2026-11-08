package assetsql

import (
	_ "embed"
	"encoding/json"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// Authored exemptions: creation outfits and the six rebirth cape series.
// Seed once; administrator edits to the typed item column survive restarts.
//
//go:embed indestructible_equipment_defaults.json
var indestructibleEquipmentDefaults []byte

func indestructibleEquipmentIDs() ([]uint16, error) {
	var ids []uint16
	err := json.Unmarshal(indestructibleEquipmentDefaults, &ids)
	return ids, err
}
func seedIndestructibleEquipment(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&NativeItemsRow{}) {
		return nil
	}
	ids, err := indestructibleEquipmentIDs()
	if err != nil {
		return err
	}
	return tx.Model(&NativeItemsRow{}).Where("native_items_key IN ?", ids).Update("value_definition_indestructible", true).Error
}
func initializeIndestructibleEquipment(c *assets.Catalog) error {
	ids, err := indestructibleEquipmentIDs()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if item, ok := c.NativeItems[id]; ok {
			item.Definition.Indestructible = true
			c.NativeItems[id] = item
			c.Items[id] = item.Definition
		}
	}
	return nil
}
