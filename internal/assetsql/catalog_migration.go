package assetsql

import (
	"fmt"
	"gorm.io/gorm"
	"reflect"
	"strings"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

const catalogSchemaVersion = 11
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

		// Add dimensions before older upgrades read the expanded typed item row.
		// Seed from retained SQL record bytes only; no external JSON/native reads.
		if schema.Version < 11 {
			for _, column := range []string{"ValueDefinitionCellWidth", "ValueDefinitionCellHeight"} {
				if !tx.Migrator().HasColumn(&NativeItemsRow{}, column) {
					if err := tx.Migrator().AddColumn(&NativeItemsRow{}, column); err != nil {
						return err
					}
				}
			}
			var rows []NativeItemsRow
			if err := tx.Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				if row.ValueDefinitionCellWidth != 0 || row.ValueDefinitionCellHeight != 0 {
					continue
				}
				var item assets.NativeItem
				if len(row.ValueRecord) != len(item.Record) {
					return fmt.Errorf("item %d: invalid retained record for inventory dimensions", row.NativeItemsKey)
				}
				copy(item.Record[:], row.ValueRecord)
				item.InitializeInventoryDimensions()
				if err := tx.Model(&NativeItemsRow{}).Where("native_items_key = ?", row.NativeItemsKey).Updates(map[string]any{
					"value_definition_cell_width":  item.Definition.CellWidth,
					"value_definition_cell_height": item.Definition.CellHeight,
				}).Error; err != nil {
					return err
				}
			}
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
		if schema.Version < 3 {
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
		}
		if schema.Version < 4 {
			for _, model := range catalogTables() {
				if named, ok := model.(interface{ TableName() string }); ok && strings.HasPrefix(named.TableName(), "catalog_arcades") {
					if err := tx.Migrator().CreateTable(model); err != nil {
						return err
					}
				}
			}
			nativeItems, err := readNativeItems(tx)
			if err != nil {
				return err
			}
			items := map[uint16]game.ItemDefinition{}
			for id, item := range nativeItems {
				items[id] = item.Definition
			}
			arcades, err := defaultArcades(items)
			if err != nil {
				return err
			}
			if err = writeArcades(tx, arcades); err != nil {
				return err
			}
			if err = tx.Migrator().AddColumn(&catalogPresence{}, "Arcades"); err != nil {
				return err
			}
			if err = tx.Model(&catalogPresence{}).Where("id = ?", catalogMetadataID).Update("arcades", true).Error; err != nil {
				return err
			}
		}
		if schema.Version < 5 {
			for _, model := range catalogTables() {
				if named, ok := model.(interface{ TableName() string }); ok && strings.HasPrefix(named.TableName(), "catalog_terrains") {
					if err := tx.Migrator().CreateTable(model); err != nil {
						return err
					}
				}
			}
			terrain, err := importedTerrains(tx)
			if err != nil {
				return err
			}
			if err = writeTerrains(tx, terrain); err != nil {
				return err
			}
			if err = tx.Migrator().AddColumn(&catalogPresence{}, "Terrains"); err != nil {
				return err
			}
			if err = tx.Model(&catalogPresence{}).Where("id = ?", catalogMetadataID).Update("terrains", true).Error; err != nil {
				return err
			}
		}
		if schema.Version < 6 {
			if !tx.Migrator().HasColumn(&SkillsRow{}, "ValueTargetingPresent") {
				if err := tx.Migrator().AddColumn(&SkillsRow{}, "ValueTargetingPresent"); err != nil {
					return err
				}
			}
			for _, model := range catalogTables() {
				if named, ok := model.(interface{ TableName() string }); ok && strings.HasPrefix(named.TableName(), "catalog_skills_targeting") && !tx.Migrator().HasTable(model) {
					if err := tx.Migrator().CreateTable(model); err != nil {
						return err
					}
				}
			}
		}

		if schema.Version < 7 {
			missing := !tx.Migrator().HasTable(&FishingRow{})
			for _, model := range catalogTables() {
				if named, ok := model.(interface{ TableName() string }); ok && strings.HasPrefix(named.TableName(), "catalog_fishing") && !tx.Migrator().HasTable(model) {
					if err := tx.Migrator().CreateTable(model); err != nil {
						return err
					}
				}
			}
			if missing {
				native, err := readNativeItems(tx)
				if err != nil {
					return err
				}
				items := map[uint16]game.ItemDefinition{}
				for id, n := range native {
					items[id] = n.Definition
				}
				f, err := defaultFishing(items)
				if err != nil {
					return err
				}
				if err = writeFishing(tx, f); err != nil {
					return err
				}
			}
		}
		if schema.Version < 8 {
			if err := migrateFishingWeights(tx); err != nil {
				return err
			}
		}

		if schema.Version < 9 {
			native, err := readNativeItems(tx)
			if err != nil {
				return err
			}
			items := map[uint16]game.ItemDefinition{}
			for id, n := range native {
				items[id] = n.Definition
			}
			for _, model := range catalogTables() {
				if named, ok := model.(interface{ TableName() string }); ok && (strings.HasPrefix(named.TableName(), "catalog_manufacturing") || strings.HasPrefix(named.TableName(), "catalog_reborn_classes")) && !tx.Migrator().HasTable(model) {
					if err := tx.Migrator().CreateTable(model); err != nil {
						return err
					}
				}
			}
			for _, column := range []string{"Manufacturing", "RebornClasses"} {
				if !tx.Migrator().HasColumn(&catalogPresence{}, column) {
					if err := tx.Migrator().AddColumn(&catalogPresence{}, column); err != nil {
						return err
					}
				}
			}
			var count int64
			if err = tx.Model(&ManufacturingRow{}).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				formulas, err := importedManufacturing(tx, items)
				if err != nil {
					return err
				}
				if err = writeManufacturing(tx, formulas); err != nil {
					return err
				}
			}
			if err = tx.Model(&RebornClassesRow{}).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				classes, err := defaultRebornClasses(items)
				if err != nil {
					return err
				}
				if err = writeRebornClasses(tx, classes); err != nil {
					return err
				}
			}
			if err = tx.Model(&catalogPresence{}).Where("id = ?", catalogMetadataID).Updates(map[string]any{"manufacturing": true, "reborn_classes": true}).Error; err != nil {
				return err
			}
			for _, column := range []struct {
				Model any
				Name  string
			}{
				{&EconomyManufacturingRow{}, "ValueSuccessPercentPresent"}, {&EconomyManufacturingRow{}, "ValueSuccessPercent"}, {&EconomyManufacturingRow{}, "ValueFee"}, {&EconomySynthesisRatesRow{}, "ValueFee"},
			} {
				if !tx.Migrator().HasColumn(column.Model, column.Name) {
					if err := tx.Migrator().AddColumn(column.Model, column.Name); err != nil {
						return err
					}
				}
			}
		}
		if schema.Version < 9 {
			economy, err := readEconomy(tx)
			if err != nil {
				return err
			}
			if err = applyAuthoredSynthesis(tx, &economy); err != nil {
				return err
			}
			if err = tx.Where("economy_key = ?", catalogMetadataID).Delete(&EconomyRow{}).Error; err != nil {
				return err
			}
			if err = writeEconomy(tx, economy); err != nil {
				return err
			}
		}

		if schema.Version < 10 {
			for _, model := range catalogTables() {
				if named, ok := model.(interface{ TableName() string }); ok && strings.HasPrefix(named.TableName(), "catalog_quest_definitions") {
					if !tx.Migrator().HasTable(model) {
						if err := tx.Migrator().CreateTable(model); err != nil {
							return err
						}
					}
				}
			}
			if !tx.Migrator().HasColumn(&catalogPresence{}, "QuestDefinitions") {
				if err := tx.Migrator().AddColumn(&catalogPresence{}, "QuestDefinitions"); err != nil {
					return err
				}
			}
			var count int64
			if err := tx.Model(&QuestDefinitionsRow{}).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				marks, err := readMarks(tx)
				if err != nil {
					return err
				}
				defs, err := importedQuestDefinitions(tx, marks)
				if err != nil {
					return err
				}
				if err = writeQuestDefinitions(tx, defs); err != nil {
					return err
				}
			}
			if err := tx.Model(&catalogPresence{}).Where("id = ?", catalogMetadataID).Update("quest_definitions", true).Error; err != nil {
				return err
			}
		}
		return tx.Model(&schema).Update("version", catalogSchemaVersion).Error
	}
	c, err := loadLegacyTransaction(tx)
	if err != nil {
		return err
	}
	c.Terrains, err = importedTerrains(tx)
	if err != nil {
		return err
	}
	c.Arcades, err = defaultArcades(c.Items)
	if err != nil {
		return err
	}
	c.Tents, err = defaultTents()
	if err != nil {
		return err
	}
	c.QuestDefinitions, err = importedQuestDefinitions(tx, c.Marks)
	if err != nil {
		return err
	}
	c.Manufacturing, err = importedManufacturing(tx, c.Items)
	if err != nil {
		return err
	}
	c.RebornClasses, err = defaultRebornClasses(c.Items)
	if err != nil {
		return err
	}
	c.Fishing, err = defaultFishing(c.Items)
	if err != nil {
		return err
	}
	c.Economy, err = defaultEconomy()
	if err != nil {
		return err
	}
	if err = applyAuthoredSynthesis(tx, &c.Economy); err != nil {
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
	if err = assets.ValidateManufacturing(c.Manufacturing, c.Items); err != nil {
		return nil, err
	}
	if err = assets.ValidateTerrains(c.Terrains); err != nil {
		return nil, err
	}
	if err = assets.ValidateArcades(c.Arcades, c.Items); err != nil {
		return nil, err
	}
	if err = validateTents(c.Tents); err != nil {
		return nil, err
	}
	if err = validateFishingCatalog(c); err != nil {
		return nil, err
	}
	if err = validateEconomy(c.Economy); err != nil {
		return nil, err
	}
	if len(c.NativeItems) == 0 || len(c.NPCs) == 0 || len(c.Skills) == 0 || len(c.Maps) == 0 {
		return nil, fmt.Errorf("required structured definitions are empty")
	}
	if err = validateDefinition("RebornClasses", c); err != nil {
		return nil, err
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
	if err = assets.ValidateQuestDefinitions(c); err != nil {
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
