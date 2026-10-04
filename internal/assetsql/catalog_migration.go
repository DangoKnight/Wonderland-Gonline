package assetsql

import (
	"fmt"
	"gorm.io/gorm"
	"reflect"
	"strings"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
)

const catalogSchemaVersion = 3
const catalogWriteBatch = 10
const catalogMetadataID = 1

type catalogSchema struct {
	ID      int `gorm:"primaryKey;autoIncrement:false"`
	Version int
}

func (catalogSchema) TableName() string { return "catalog_schema" }

// MigrateDatabase creates an additive projection of existing SQL definitions.
// Source records remain untouched for provenance and compatibility administration.
// Call explicitly on an offline copy before deploying an upgraded database.
func MigrateDatabase(path string) error {
	db, err := assetdb.Open(path)
	if err != nil {
		return err
	}
	defer assetdb.Close(db)
	return db.Transaction(func(tx *gorm.DB) error { return migrateCatalog(tx) })
}
func migrateCatalog(tx *gorm.DB) error {
	if tx.Migrator().HasTable(&catalogSchema{}) {
		var schema catalogSchema
		if err := tx.First(&schema, catalogMetadataID).Error; err != nil {
			return err
		}
		if schema.Version == catalogSchemaVersion {
			return nil
		}
		if schema.Version < 1 || schema.Version > catalogSchemaVersion {
			return fmt.Errorf("unsupported structured asset schema %d", schema.Version)
		}
		if schema.Version == 1 {
			for _, model := range catalogTables() {
				named, ok := model.(interface{ TableName() string })
				if ok && strings.HasPrefix(named.TableName(), "catalog_economy") {
					if err := tx.Migrator().CreateTable(model); err != nil {
						return err
					}
				}
			}
			economy, err := defaultEconomy()
			if err != nil {
				return err
			}
			if err = writeEconomy(tx, economy); err != nil {
				return err
			}
		}
		// v3 adds only typed tent rules and default furniture; preserve existing data.
		for _, model := range catalogTables() {
			named, ok := model.(interface{ TableName() string })
			if ok && strings.HasPrefix(named.TableName(), "catalog_tents") {
				if err := tx.Migrator().CreateTable(model); err != nil {
					return err
				}
			}
		}
		tents, err := defaultTents()
		if err != nil {
			return err
		}
		if err = writeTents(tx, tents); err != nil {
			return err
		}

		return tx.Model(&schema).Update("version", catalogSchemaVersion).Error
	}
	c, err := loadLegacyTransaction(tx)
	if err != nil {
		return err
	}
	c.Tents, err = defaultTents()
	if err != nil {
		return err
	}
	c.Economy, err = defaultEconomy()
	if err != nil {
		return err
	}
	for _, model := range append(catalogTables(), &catalogSchema{}) {
		if err := tx.Migrator().CreateTable(model); err != nil {
			return err
		}
	}
	if err = writeCatalog(tx, c); err != nil {
		return err
	}
	// Reading the projection before committing also verifies ordered children,
	// singleton rows and fixed-length protocol metadata.
	projected, err := readCatalog(tx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(c, projected) {
		return fmt.Errorf("structured asset conversion changed catalog values; migration rolled back")
	}
	return tx.Create(&catalogSchema{ID: catalogMetadataID, Version: catalogSchemaVersion}).Error
}

// LoadTransaction reads only structured definitions; it never assembles source
// documents or reconstructs native files. Catalog snapshots are SQL-derived.
func LoadTransaction(tx *gorm.DB) (*assets.Catalog, error) {
	var schema catalogSchema
	if err := tx.First(&schema, catalogMetadataID).Error; err != nil {
		return nil, fmt.Errorf("structured assets unavailable; run database-migrate: %w", err)
	}
	if schema.Version != catalogSchemaVersion {
		return nil, fmt.Errorf("unsupported structured asset schema %d", schema.Version)
	}
	c, err := readCatalog(tx)
	if err != nil {
		return nil, err
	}
	if err = validateTents(c.Tents); err != nil {
		return nil, err
	}
	if err = validateEconomy(c.Economy); err != nil {
		return nil, err
	}
	if len(c.NativeItems) == 0 || len(c.NPCs) == 0 || len(c.Skills) == 0 || len(c.Maps) == 0 {
		return nil, fmt.Errorf("required structured definitions are empty")
	}
	if err = validateDefinition("Skills", c); err != nil {
		return nil, err
	}
	if err = validateDefinition("LuckyDraw", c); err != nil {
		return nil, err
	}
	if err = validateDefinition("CombatTrials", c); err != nil {
		return nil, err
	}
	if err = validateDefinition("QuestVisibility", c); err != nil {
		return nil, err
	}
	if err = validateDefinition("ChestPools", c); err != nil {
		return nil, err
	}
	return c, nil
}

// SaveEditedCatalog commits the validated administrator candidate.
func SaveEditedCatalog(tx *gorm.DB, c *assets.Catalog) error { return writeCatalog(tx, c) }
