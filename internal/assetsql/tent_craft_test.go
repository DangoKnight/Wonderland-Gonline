package assetsql

import (
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/assetdb"
)

func TestTentCraftMigrationPreservesValuesAndEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.db")
	if _, err := BuildRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.Migrator().DropColumn(&ManufacturingRow{}, "ValueDefaultDurationSeconds"); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropColumn(&NativeItemsRow{}, "ValueDefinitionTentFloorSwitch"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ManufacturingRow{}).Where("manufacturing_key = ?", 511).Update("value_duration_seconds", 77).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 64).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	var recipe ManufacturingRow
	if err := db.First(&recipe, "manufacturing_key = ?", 511).Error; err != nil {
		t.Fatal(err)
	}
	if recipe.ValueDurationSeconds != 77 || recipe.ValueDefaultDurationSeconds != 77 {
		t.Fatal("migration changed the existing duration", recipe)
	}
	var stairs NativeItemsRow
	if err := db.First(&stairs, "native_items_key = ?", 39031).Error; err != nil || !stairs.ValueDefinitionTentFloorSwitch {
		t.Fatal("stairs flag not seeded", err)
	}
	if err := db.Model(&ManufacturingRow{}).Where("manufacturing_key = ?", 511).Update("value_duration_seconds", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 39031).Update("value_definition_tent_floor_switch", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	defaults, err := DefaultManufactureDurations(db)
	if err != nil || defaults[511] != 77 {
		t.Fatal("duration edit destroyed its reset baseline", defaults[511], err)
	}
	stairs = NativeItemsRow{}
	if err := db.First(&stairs, "native_items_key = ?", 39031).Error; err != nil || stairs.ValueDefinitionTentFloorSwitch {
		t.Fatal("migration overwrote administrator flags", err)
	}
}
