package assetsql

import (
	"testing"
	"wonderland-gonline/internal/assetdb"
)

func TestCaptureMigrationPreservesTypedEdits(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Model(&NPCsRow{}).Where("n_p_cs_key = ?", 42).Update("value_name", "Edited monster").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&assetdb.Record{Asset: "npc.dat", Collection: "records", Ordinal: 9999, JSON: `{"fields":{"id":42,"unknown_u8_offset_34":1}}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropColumn(&NPCsRow{}, "ValueCaptureRestriction"); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn(&NPCsRow{}, "ValueCaptureRestriction") {
		t.Fatal("fixture failed to drop capture column")
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", 1).Update("version", 16).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	var row NPCsRow
	// Open a fresh reader, as deployment does; a live connection can cache the
	// old SELECT * column layout across SQLite's table rebuild.
	assetdb.Close(db)
	db, err = assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.First(&row, "n_p_cs_key = ?", 42).Error; err != nil {
		t.Fatal(err)
	}
	if row.ValueName != "Edited monster" || row.ValueCaptureRestriction != 1 {
		t.Fatal("migration lost definitions", row)
	}
	if err = db.Model(&NPCsRow{}).Where("n_p_cs_key = ?", 42).Update("value_capture_restriction", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	if err = db.First(&row, "n_p_cs_key = ?", 42).Error; err != nil {
		t.Fatal(err)
	}
	if row.ValueCaptureRestriction != 0 {
		t.Fatal("repeat migration overwrote edit")
	}
}
