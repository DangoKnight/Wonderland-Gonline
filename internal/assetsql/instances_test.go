package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestInstanceCatalogMigrationAndSQLAuthority(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	row := assetdb.Record{Asset: "scenedata.dat", Collection: "records", Ordinal: 1, JSON: `{"name":{"text":"Trial"},"fields":{"id":30001,"unknown_u16_offset_38":30001,"unknown_u8_offset_96":14,"unknown_u8_offset_97":0,"unknown_u16_offset_101":40001,"unknown_u16_offset_103":50,"unknown_u16_offset_105":40,"unknown_u16_offset_107":900,"unknown_u16_offset_109":800}}`}
	if err = db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil || len(c.Instances) != 1 || c.Instances[0].Capacity != 14 {
		t.Fatal(c, err)
	}
	if err = db.Model(&InstancesRow{}).Where("instances_ordinal = ?", 0).Update("value_minimum_level", 25).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Delete(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err = LoadDatabase(path)
	if err != nil || c.Instances[0].MinimumLevel != 25 {
		t.Fatal("runtime read source instead of SQL", err)
	}
	// Replaying a schema-11 installation preserves all existing catalog edits.
	if err = db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 11).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
}
