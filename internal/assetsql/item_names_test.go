package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestItemNameMigrationPreservesEditsAndMetadata(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	for _, row := range []NativeItemsRow{
		{NativeItemsKey: 10034, ValueDefinitionID: 10034, ValueDefinitionName: "Pure Iron Knif", ValueIcon: 42, ValueDescription: "Keep description", ValueRecord: make([]byte, 451)},
		{NativeItemsKey: 10036, ValueDefinitionID: 10036, ValueDefinitionName: "Custom sword", ValueRecord: make([]byte, 451)},
	} {
		if err := db.Save(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 22).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := MigrateDatabase(path); err != nil {
			t.Fatal(err)
		}
	}
	var row NativeItemsRow
	if err := db.First(&row, "native_items_key = ?", 10034).Error; err != nil || row.ValueDefinitionName != "Pure Iron Knife" || row.ValueIcon != 42 || row.ValueDescription != "Keep description" {
		t.Fatal(row, err)
	}
	row = NativeItemsRow{}
	if err := db.First(&row, "native_items_key = ?", 10036).Error; err != nil || row.ValueDefinitionName != "Custom sword" {
		t.Fatal(row, err)
	}
}
