package assetsql

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestAccountGroundMigrationPreservesAbsentStory(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "partial.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.Transaction(migrateAccountGroundStory); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable(&StoryRow{}) {
		t.Fatal("migration initialized an absent dataset")
	}
}

func TestAccountGroundPolicyMigrationPreservesEdits(t *testing.T) {
	before := assets.DefaultStoryDefinitions()
	before.Rules = slices.DeleteFunc(before.Rules, func(r assets.StoryRule) bool { return r.Kind == assets.StoryAccountGroundOnce })
	before.Rules[0].Text = "Administrator edit"
	db := storyReadFixture(t, before)
	if err := db.Transaction(migrateAccountGroundStory); err != nil {
		t.Fatal(err)
	}
	after, err := readStory(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Rules) != len(before.Rules)+1 || !reflect.DeepEqual(before.Rules, after.Rules[:len(before.Rules)]) {
		t.Fatal("existing rules overwritten")
	}
	if err := db.Transaction(migrateAccountGroundStory); err != nil {
		t.Fatal(err)
	}
	repeated, err := readStory(db)
	if err != nil || !reflect.DeepEqual(after, repeated) {
		t.Fatal("migration not repeatable", err)
	}
}
