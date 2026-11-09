package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/game"
)

// The Astrology lady's native event grants the Notepad. Correct only starter
// definitions; existing characters and the original import evidence are retained.
func removeStarterNotepad(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&StarterItemsRow{}) {
		return nil
	}
	var rows []StarterItemsRow
	if err := tx.Order("starter_items_ordinal").Find(&rows).Error; err != nil {
		return err
	}
	retained := make([]game.StarterGrant, 0, len(rows))
	for _, row := range rows {
		if row.ValueID != game.NotepadItemID {
			retained = append(retained, game.StarterGrant{ID: row.ValueID, Count: row.ValueCount})
		}
	}
	if len(retained) == len(rows) {
		return nil
	}
	// Catalog readers require contiguous ordinals after removing a definition.
	if err := tx.Where("1 = 1").Delete(&StarterItemsRow{}).Error; err != nil {
		return err
	}
	return writeStarterItems(tx, retained)
}
