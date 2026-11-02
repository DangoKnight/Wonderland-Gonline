package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// v15 seeds story policies exactly once. Existing edits and deliberate empty
// definitions remain authoritative, including on a repeated offline migration.
func migrateStory(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_story", "", func() error {
		defs := assets.DefaultStoryDefinitions()
		if err := assets.ValidateStoryDefinitions(defs); err != nil {
			return err
		}
		return writeStory(tx, defs)
	})
}
