package assetsql

import (
	"reflect"
	"slices"
	"testing"
	"wonderland-gonline/internal/assets"
)

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
