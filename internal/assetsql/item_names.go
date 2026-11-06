package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

func migrateItemNames(tx *gorm.DB) error {
	patches, err := assets.ItemNamePatches()
	if err != nil {
		return err
	}
	for id, patch := range patches {
		if err := tx.Model(&NativeItemsRow{}).Where("native_items_key = ? AND value_definition_name = ?", id, patch.OriginalName).Update("value_definition_name", patch.Name).Error; err != nil {
			return err
		}
	}
	return nil
}

func initializeItemNames(c *assets.Catalog) error {
	if err := assets.ApplyItemNamePatches(c.NativeItems); err != nil {
		return err
	}
	for id, item := range c.NativeItems {
		if definition, ok := c.Items[id]; ok {
			definition.Name = item.Definition.Name
			c.Items[id] = definition
		}
	}
	return nil
}
