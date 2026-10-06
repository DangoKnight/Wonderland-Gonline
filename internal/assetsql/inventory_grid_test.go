package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestInventoryDimensionsV10MigrationUsesSQLAndPreservesEdits(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	var row NativeItemsRow
	if err = db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	row.ValueRecord[406], row.ValueRecord[407] = 4, 3
	if err = db.Model(&row).Updates(map[string]any{"value_record": row.ValueRecord, "value_definition_name": "Retained SQL edit"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"ValueDefinitionCellWidth", "ValueDefinitionCellHeight"} {
		if err = db.Migrator().DropColumn(&NativeItemsRow{}, col); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 10).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Where("1 = 1").Delete(&assetdb.Document{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	def := catalog.Items[row.NativeItemsKey]
	if def.CellWidth != 4 || def.CellHeight != 3 || def.Name != "Retained SQL edit" {
		t.Fatal("typed footprint migration lost SQL data", def)
	}
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", row.NativeItemsKey).Updates(map[string]any{"value_definition_cell_width": 2, "value_definition_cell_height": 2}).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	catalog, err = LoadDatabase(path)
	if err != nil || catalog.Items[row.NativeItemsKey].CellWidth != 2 || catalog.Items[row.NativeItemsKey].CellHeight != 2 {
		t.Fatal("restart overwrote dimensions", err)
	}
}
