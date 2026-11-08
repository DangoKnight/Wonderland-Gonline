package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestEquipmentDurabilityMigrationPreservesEdits(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.Migrator().DropColumn(&NativeItemsRow{}, "ValueDefinitionIndestructible"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 63).Error; err != nil {
		t.Fatal(err)
	}
	assetdb.Close(db) // Offline migrations reopen readers after changing columns.
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err = assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	var row NativeItemsRow
	if err := db.First(&row, "native_items_key = ?", 10002).Error; err != nil || !row.ValueDefinitionIndestructible {
		t.Fatal(row, err)
	}
	if err := db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_indestructible", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&row, "native_items_key = ?", 10002).Error; err != nil || row.ValueDefinitionIndestructible {
		t.Fatal("administrator edit overwritten", err)
	}
}
