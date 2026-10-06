package assetsql

import (
	"gorm.io/gorm"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestStructuredCatalogMigrationRoundTripAndAuthority(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	var original *assets.Catalog
	err = db.Transaction(func(tx *gorm.DB) error { var err error; original, err = loadLegacyTransaction(tx); return err })
	if err != nil {
		t.Fatal(err)
	}
	// Exercise nested effects, optional booleans, ordered branches and opaque bytes.
	flag := false
	s := original.Skills[23]
	s.AreaAttack = &flag
	s.Targeting = []assets.SkillTargeting{{MinGrade: 1, MaxGrade: 5, Offsets: []assets.FormationOffset{{}, {X: 1}}}, {MinGrade: 6, MaxGrade: 10, All: true}}
	s.EffectRefs = []string{}
	original.Skills[23] = s
	m := original.Maps[10017]
	m.NPCs = []assets.MapNPC{{ClickID: 8, WalkSteps: []assets.WalkStep{{X: 1, Y: 2, Delay: 3}}, Events: []byte{1, 2}, Links: []byte{}}}
	m.Events = []assets.Event{{ClickID: 8, Branches: []assets.Branch{{Index: 1, Condition: [21]byte{9}, Operations: []assets.Operation{{Index: 2, Data: [21]byte{7, 8}}}}}}}
	original.Maps[10017] = m
	if err = db.Transaction(func(tx *gorm.DB) error {
		if err := migrateCatalog(tx); err != nil {
			return err
		}
		return writeCatalog(tx, original)
	}); err != nil {
		t.Fatal(err)
	}
	var got *assets.Catalog
	if err = db.Transaction(func(tx *gorm.DB) error { var err error; got, err = LoadTransaction(tx); return err }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, got) {
		t.Fatalf("catalog round trip differs\noriginal=%#v\ngot=%#v", original, got)
	}
	// Delete the legacy source documents: gameplay must still use typed rows.
	if err = db.Where("1 = 1").Delete(&assetdb.Document{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Typed SQL edit").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err = LoadDatabase(path)
	if err != nil || got.Items[10002].Name != "Typed SQL edit" {
		t.Fatalf("SQL authority lost: %v %v", got, err)
	}
	var foreignKeys []map[string]any
	if err = db.Raw("PRAGMA foreign_key_list(catalog_maps_events)").Scan(&foreignKeys).Error; err != nil || len(foreignKeys) == 0 {
		t.Fatal("missing event ownership constraint", err)
	}
	if err = db.Where("maps_key = ?", 10017).Delete(&MapsRow{}).Error; err != nil {
		t.Fatal(err)
	}
	var children int64
	if err = db.Model(&MapsEventsBranchesOperationsRow{}).Count(&children).Error; err != nil || children != 0 {
		t.Fatal("event operation cascade failed", children, err)
	}
	var schema catalogSchema
	if err = db.First(&schema, catalogMetadataID).Error; err != nil || schema.Version != catalogSchemaVersion {
		t.Fatal(schema, err)
	}
}

func TestStructuredCatalogMigrationRollback(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Model(&assetdb.Document{}).Where("asset = ?", "item.dat").Update("json", "invalid").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err == nil {
		t.Fatal("invalid source migrated")
	}
	if db.Migrator().HasTable(&catalogSchema{}) || db.Migrator().HasTable(&NativeItemsRow{}) {
		t.Fatal("partial schema committed")
	}
}

func TestStructuredAdministrationPreservesOtherEditsAndRejectsInvalidData(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Independent SQL edit").Error; err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		c, err := DefinitionCandidate(tx, "Mall", []byte(`[{"item_id":10002,"item_name":"Changed mall","count":2,"point_cost":99}]`), nil)
		if err != nil {
			return err
		}
		return SaveEditedCatalog(tx, c)
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil || c.Items[10002].Name != "Independent SQL edit" || c.Mall[0].Cost != 99 {
		t.Fatal("unrelated changes lost", c, err)
	}
	id := 42
	if err = db.Transaction(func(tx *gorm.DB) error {
		_, err := DefinitionCandidate(tx, "NPCs", []byte(`{"id":43}`), &id)
		return err
	}); err == nil {
		t.Fatal("changed NPC identity")
	}
	if err = db.Transaction(func(tx *gorm.DB) error {
		_, err := DefinitionCandidate(tx, "Mall", []byte(`[{"item_id":999,"count":1}]`), nil)
		return err
	}); err == nil {
		t.Fatal("unknown mall item accepted")
	}
	if err = db.Transaction(func(tx *gorm.DB) error {
		_, err := DefinitionCandidate(tx, "Mall", []byte(`[{"item_id":10002,"count":1,"typo":2}]`), nil)
		return err
	}); err == nil {
		t.Fatal("unknown field accepted")
	}
	// Administration remains operational after legacy migration source rows disappear.
	if err = db.Where("1 = 1").Delete(&assetdb.Document{}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error {
		raw, err := ReadDefinitionRecord(tx, "NPCs", 42)
		if err != nil {
			return err
		}
		candidate, err := DefinitionCandidate(tx, "NPCs", raw, &id)
		if err != nil {
			return err
		}
		return SaveEditedCatalog(tx, candidate)
	}); err != nil {
		t.Fatal(err)
	}
}
