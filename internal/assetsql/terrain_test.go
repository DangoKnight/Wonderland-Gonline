package assetsql

import (
	"encoding/json"
	"gorm.io/gorm"
	"reflect"
	"testing"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
)

func TestTerrainSchemaV4UpgradePreservesDefinitions(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Migrator().DropTable(&TerrainsRow{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropColumn(&catalogPresence{}, "Terrains"); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 4).Error; err != nil {
		t.Fatal(err)
	}
	record := assetdb.Record{Asset: "ground.mmg", Collection: "entries", Ordinal: 0, JSON: `{"name":"10017.map","terrain":{"width":40,"height":40,"grid_width":2,"grid_height":2,"cells_hex":"00170000"}}`}
	if err = db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Preserved item edit").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	want := assets.Terrain{Width: 40, Height: 40, GridWidth: 2, GridHeight: 2, Cells: []byte{0, 23, 0, 0}}
	if !reflect.DeepEqual(got.Terrains[10017], want) || got.Items[10002].Name != "Preserved item edit" {
		t.Fatal("projection or edit changed", got.Terrains, got.Items[10002])
	}
	// Typed SQL remains authoritative after provenance is deleted.
	if err = db.Where("asset = ?", "ground.mmg").Delete(&assetdb.Record{}).Error; err != nil {
		t.Fatal(err)
	}
	got.Terrains[10017] = assets.Terrain{Width: 20, Height: 20, GridWidth: 1, GridHeight: 1, Cells: []byte{0}}
	if err = db.Transaction(func(tx *gorm.DB) error { return SaveEditedCatalog(tx, got) }); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	next, err := LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(got.Terrains, next.Terrains) {
		t.Fatal("migration overwrote edit", err)
	}
	raw, _ := json.Marshal(map[uint16]assets.Terrain{1: {Width: 20, Height: 20, GridWidth: 1, GridHeight: 1, Cells: []byte{}}})
	if _, err = DefinitionCandidate(db, "Terrains", raw, nil); err == nil {
		t.Fatal("invalid geometry edit accepted")
	}
}
func TestTerrainMalformedImportRollsBack(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Migrator().DropTable(&TerrainsRow{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropColumn(&catalogPresence{}, "Terrains"); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 4).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Create(&assetdb.Record{Asset: "ground.mmg", Collection: "entries", Ordinal: 0, JSON: `{"name":"1.map","terrain":{"width":20,"height":20,"grid_width":1,"grid_height":1,"cells_hex":""}}`}).Error
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err == nil {
		t.Fatal("missing cells accepted")
	}
	var schema catalogSchema
	if err = db.First(&schema, catalogMetadataID).Error; err != nil {
		t.Fatal(err)
	}
	if schema.Version != 4 || db.Migrator().HasTable(&TerrainsRow{}) || db.Migrator().HasColumn(&catalogPresence{}, "Terrains") {
		t.Fatal("failed migration changed schema", schema)
	}
}
