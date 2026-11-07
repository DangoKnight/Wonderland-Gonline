package assetsql

import (
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/assetdb"
)

func TestSkillContactsMigrationPreservesEditsAndRollsBack(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&SkillsRow{}, &catalogSchema{}, &catalogPresence{}, &assetdb.Record{}); err != nil {
		t.Fatal(err)
	}
	rows := []SkillsRow{{SkillsKey: 11016, ValueID: 11016, ValueName: "Edited name", ValueStatMultiplier: 1.2}, {SkillsKey: 11166, ValueID: 11166, ValueContactEvents: 7}, {SkillsKey: 65535, ValueID: 65535}}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&catalogSchema{ID: catalogMetadataID, Version: 54}).Error; err != nil {
		t.Fatal(err)
	}
	// No original MBTM document or record is present. SQL failure must still
	// roll back counts and version together.
	if err := db.Exec("CREATE TRIGGER fail_contacts BEFORE UPDATE OF value_contact_events ON catalog_skills BEGIN SELECT RAISE(ABORT, 'forced contact failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err == nil {
		t.Fatal("failed SQL migration accepted")
	}
	var schema catalogSchema
	db.First(&schema, catalogMetadataID)
	var got SkillsRow
	db.First(&got, "skills_key = ?", 11016)
	if schema.Version != 54 || got.ValueContactEvents != 0 {
		t.Fatal("failed migration changed schema or counts", schema, got)
	}
	if err := db.Exec("DROP TRIGGER fail_contacts").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	loaded, err := readSkills(db)
	if err != nil || loaded[11016].ContactEvents != 2 || loaded[11016].Name != "Edited name" || loaded[11016].StatMultiplier != 1.2 || loaded[11166].ContactEvents != 7 || loaded[65535].ContactEvents != 0 {
		t.Fatal("contact migration lost edits or invented missing data", loaded, err)
	}
	if err := db.Model(&SkillsRow{}).Where("skills_key = ?", 11016).Update("value_contact_events", 5).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	loaded, err = readSkills(db)
	if err != nil || loaded[11016].ContactEvents != 5 {
		t.Fatal("repeated initialization overwrote count", loaded, err)
	}
}

func TestSkillContactColumnUpgradeFromMissingColumn(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "old-contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&SkillsRow{}, &catalogSchema{}, &catalogPresence{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&SkillsRow{SkillsKey: 11016, ValueID: 11016, ValueName: "Keep"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropColumn(&SkillsRow{}, "ValueContactEvents"); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&catalogSchema{ID: catalogMetadataID, Version: 54}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	loaded, err := readSkills(db)
	if err != nil || loaded[11016].Name != "Keep" || loaded[11016].ContactEvents != 2 {
		t.Fatal("old schema upgrade", loaded, err)
	}
}
