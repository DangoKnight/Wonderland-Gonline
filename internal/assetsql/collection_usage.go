package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// Exact historical importer diagnostic. Operator reasons, combined diagnostics
// and absent disable records must never be replaced by this content migration.
const legacyCollectionDisableReason = "gathering_pool_mapping_and_weights_missing;unsupported_condition_type"

func initializeCollectionUsage(c *assets.Catalog) {
	for _, key := range assets.AvailableCollectionEvents(c, legacyCollectionDisableReason) {
		delete(c.DisabledEvents, key)
	}
}

func migrateCollectionUsage(tx *gorm.DB) error {
	// Read only the roots this historical migration owns/depends on. Reading
	// the current complete catalog requires tables added by later migrations.
	c := &assets.Catalog{}
	var err error
	if c.Maps, err = readMaps(tx); err != nil {
		return err
	}
	if c.DisabledEvents, err = readDisabledEvents(tx); err != nil {
		return err
	}
	if c.EventGathering, err = readEventGathering(tx); err != nil {
		return err
	}
	if c.Talks, err = readTalks(tx); err != nil {
		return err
	}
	if c.NativeItems, err = readNativeItems(tx); err != nil {
		return err
	}
	c.Items = make(map[uint16]game.ItemDefinition, len(c.NativeItems))
	for id, item := range c.NativeItems {
		c.Items[id] = item.Definition
	}
	for _, key := range assets.AvailableCollectionEvents(c, legacyCollectionDisableReason) {
		if err := tx.Where("disabled_events_key = ? AND value = ?", key, legacyCollectionDisableReason).Delete(&DisabledEventsRow{}).Error; err != nil {
			return err
		}
	}
	return nil
}
