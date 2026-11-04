package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
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
	c, err := readCatalog(tx)
	if err != nil {
		return err
	}
	for _, key := range assets.AvailableCollectionEvents(c, legacyCollectionDisableReason) {
		if err := tx.Where("disabled_events_key = ? AND value = ?", key, legacyCollectionDisableReason).Delete(&DisabledEventsRow{}).Error; err != nil {
			return err
		}
	}
	return nil
}
