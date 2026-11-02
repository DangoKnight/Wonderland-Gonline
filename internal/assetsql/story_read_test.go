package assetsql

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"

	"gorm.io/gorm"
)

func storyReadFixture(t testing.TB, defs assets.StoryDefinitions) *gorm.DB {
	t.Helper()
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "story.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { assetdb.Close(db) })
	for _, model := range catalogTables() {
		name := model.(interface{ TableName() string }).TableName()
		if name == "catalog_story" || strings.HasPrefix(name, "catalog_story_") {
			if err := db.Migrator().CreateTable(model); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return writeStory(tx, defs) }); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestStoryBatchedReadPreservesProjectionAndQueryBound(t *testing.T) {
	defs := assets.DefaultStoryDefinitions()
	defs.Rules = append(defs.Rules,
		assets.StoryRule{ID: "nil_children"},
		assets.StoryRule{ID: "empty_children", Gates: []assets.StoryGate{}, Cues: []assets.StoryCue{{Data: []uint32{}}, {Data: nil}}},
	)
	defs.Cinematics = append(defs.Cinematics, assets.StoryCinematic{ID: "siblings", Steps: []assets.StoryCinematicStep{
		{Cues: []assets.StoryCue{{Data: []uint32{1, 2}}, {Data: []uint32{3}}}},
		{Cues: []assets.StoryCue{{Data: []uint32{4, 5}}}},
	}})
	db := storyReadFixture(t, defs)
	queries := map[string]int{}
	if err := db.Callback().Query().After("gorm:query").Register("test:story_query_bound", func(tx *gorm.DB) { queries[tx.Statement.Table]++ }); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		got, err := readStory(tx)
		if err == nil && !reflect.DeepEqual(got, defs) {
			t.Error("batch changed parent ownership, order or nil/empty values")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for table, n := range queries {
		if n != 1 {
			t.Errorf("%s: %d reads; want one per required table", table, n)
		}
	}
	// Reusing the same SQL transaction after an edit must create a fresh
	// projection, and a later rollback must leave the stored definitions intact.
	rollback := errors.New("rollback")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := readStory(tx); err != nil {
			return err
		}
		if err := tx.Model(&StoryRulesRow{}).Where("story_key = ? AND story_rules_ordinal = ?", 1, 0).Update("value_id", "changed_in_transaction").Error; err != nil {
			return err
		}
		got, err := readStory(tx)
		if err != nil {
			return err
		}
		if got.Rules[0].ID != "changed_in_transaction" {
			t.Error("read reused a stale transaction cache")
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	got, err := readStory(db)
	if err != nil || !reflect.DeepEqual(got, defs) {
		t.Fatal("read escaped transaction or rollback", err)
	}
}

func TestStoryBatchedReadRetainsFailuresAndLazyChildren(t *testing.T) {
	t.Run("ordinal", func(t *testing.T) {
		defs := assets.StoryDefinitions{Rules: []assets.StoryRule{{Gates: []assets.StoryGate{{Source: "mark_exists"}}}}}
		db := storyReadFixture(t, defs)
		if err := db.Model(&StoryRulesGatesRow{}).Where("story_key = ?", 1).Update("story_rules_gates_ordinal", 2).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := readStory(db); err == nil || !strings.Contains(err.Error(), "invalid catalog ordinal") {
			t.Fatal("invalid child ordinal accepted", err)
		}
	})
	t.Run("query_failure", func(t *testing.T) {
		db := storyReadFixture(t, assets.StoryDefinitions{Rules: []assets.StoryRule{{}}})
		failure := errors.New("child query failed")
		if err := db.Callback().Query().Before("gorm:query").Register("test:story_read_failure", func(tx *gorm.DB) {
			if tx.Statement.Table == "catalog_story_rules" {
				tx.AddError(failure)
			}
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := readStory(db); !errors.Is(err, failure) {
			t.Fatal("child query error swallowed", err)
		}
	})
	t.Run("lazy_empty", func(t *testing.T) {
		db := storyReadFixture(t, assets.StoryDefinitions{})
		var tables []string
		if err := db.Callback().Query().After("gorm:query").Register("test:story_empty", func(tx *gorm.DB) { tables = append(tables, tx.Statement.Table) }); err != nil {
			t.Fatal(err)
		}
		got, err := readStory(db)
		if err != nil || !reflect.DeepEqual(got, assets.StoryDefinitions{}) || !reflect.DeepEqual(tables, []string{"catalog_story"}) {
			t.Fatal("empty story fetched unused children", tables, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := readStory(db.WithContext(ctx)); !errors.Is(err, context.Canceled) {
			t.Fatal("read dropped caller cancellation", err)
		}
	})
}
