package assetsql

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func removeStoryProjection(t *testing.T, db *gorm.DB) {
	t.Helper()
	models := catalogTables()
	for i := len(models) - 1; i >= 0; i-- {
		name := models[i].(interface{ TableName() string }).TableName()
		if name == "catalog_story" || strings.HasPrefix(name, "catalog_story_") {
			if err := db.Migrator().DropTable(models[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 14).Error; err != nil {
		t.Fatal(err)
	}
}

func TestStoryMigrationPreservesEditsAndEmptyDefinitions(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	removeStoryProjection(t, db)
	if err := db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Operator item edit").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Story, assets.DefaultStoryDefinitions()) || got.Items[10002].Name != "Operator item edit" {
		t.Fatal("story seed or unrelated definition changed")
	}
	defs := assets.StoryDefinitions{Rules: []assets.StoryRule{{ID: "operator_movie", Kind: assets.StoryMovieMode, Scope: assets.StoryScope{Maps: []uint16{10017}}, Value: 1}}}
	raw, _ := json.Marshal(defs)
	if err := db.Transaction(func(tx *gorm.DB) error {
		c, err := DefinitionCandidate(tx, "Story", raw, nil)
		if err != nil {
			return err
		}
		return SaveEditedCatalog(tx, c)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 14).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err = LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(got.Story, defs) {
		t.Fatal("migration replaced edited story", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&StoryRow{}).Error; err != nil {
			return err
		}
		return writeStory(tx, assets.StoryDefinitions{})
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 14).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err = LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(got.Story, assets.StoryDefinitions{}) {
		t.Fatal("migration refilled deliberately empty story definitions", err)
	}
}

func TestStoryMigrationRollbackAndInvalidEdit(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	before, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := assets.StoryDefinitions{Rules: []assets.StoryRule{{ID: "bad_reference", Kind: assets.StoryContinue, Scope: assets.StoryScope{Maps: []uint16{10017}, Events: []uint16{65000}}}}}
	raw, _ := json.Marshal(invalid)
	if err := db.Transaction(func(tx *gorm.DB) error { _, err := DefinitionCandidate(tx, "Story", raw, nil); return err }); err == nil {
		t.Fatal("invalid reference accepted")
	}
	got, err := LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(got.Story, before.Story) {
		t.Fatal("invalid edit changed story", err)
	}
	removeStoryProjection(t, db)
	const callback = "test:reject_story_seed"
	if err := db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_story_rules" {
			tx.AddError(errors.New("seed storage failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(callback)
	if err := db.Transaction(migrateCatalog); err == nil {
		t.Fatal("failed story seed committed")
	}
	var schema catalogSchema
	if err := db.First(&schema, catalogMetadataID).Error; err != nil || schema.Version != 14 || db.Migrator().HasTable(&StoryRow{}) {
		t.Fatal("partial story migration", schema, err)
	}
}

// Installed SQL is copied before migration; regular databases remain untouched.
func TestStoryInstalledMigration(t *testing.T) {
	source := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if source == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB to an offline SQL catalog")
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(source + suffix); !os.IsNotExist(err) {
			t.Fatal("use a closed/offline SQL catalog")
		}
	}
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	path := filepath.Join(t.TempDir(), "assets.db")
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(output, input); err != nil {
		output.Close()
		t.Fatal(err)
	}
	if err = output.Close(); err != nil {
		t.Fatal(err)
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Story, assets.DefaultStoryDefinitions()) {
		t.Fatal("installed story seed mismatch")
	}
	if err = MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	again, err := LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(again.Story, got.Story) {
		t.Fatal("repeated migration changed story", err)
	}
}
