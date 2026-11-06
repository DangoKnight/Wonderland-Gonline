package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

func migrateResourceRespawns(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_resource_respawns", "resource_respawns", func() error { return writeResourceRespawns(tx, []assets.ResourceRespawn{}) })
}
