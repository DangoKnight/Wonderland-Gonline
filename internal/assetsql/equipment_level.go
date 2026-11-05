package assetsql

import (
	"fmt"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

func migrateEquipLevel(tx *gorm.DB) error {
	const column = "ValueDefinitionEquipLevel"
	if tx.Migrator().HasColumn(&NativeItemsRow{}, column) {
		return nil
	}
	if err := tx.Migrator().AddColumn(&NativeItemsRow{}, column); err != nil {
		return err
	}
	var rows []NativeItemsRow
	if err := tx.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var item assets.NativeItem
		if len(row.ValueRecord) == 0 {
			continue
		}
		if len(row.ValueRecord) != len(item.Record) {
			return fmt.Errorf("item %d: invalid retained equipment level record", row.NativeItemsKey)
		}
		copy(item.Record[:], row.ValueRecord)
		item.InitializeEquipLevel()

		if err := tx.Model(&NativeItemsRow{}).Where("native_items_key = ?", row.NativeItemsKey).Update("value_definition_equip_level", item.Definition.EquipLevel).Error; err != nil {
			return err
		}
	}
	return nil
}
