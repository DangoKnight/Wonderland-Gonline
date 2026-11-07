package assetsql

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func mapReadFixture(t *testing.T, maps map[uint16]assets.Map) *gorm.DB {
	t.Helper()
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "maps.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { assetdb.Close(db) })
	for _, model := range catalogTables() {
		name := model.(interface{ TableName() string }).TableName()
		if name == "catalog_maps" || strings.HasPrefix(name, "catalog_maps_") {
			if err := db.Migrator().CreateTable(model); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return writeMaps(tx, maps) }); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCatalogBatchedMapsOwnershipOrderAndFreshReads(t *testing.T) {
	want := map[uint16]assets.Map{}
	for _, id := range []uint16{10, 20} {
		want[id] = assets.Map{ID: id, NPCs: []assets.MapNPC{
			{ClickID: id, Routes: map[byte][]assets.RoutePoint{1: {{X: uint32(id)}}, 2: {}, 3: nil}},
			{ClickID: id + 1, WalkSteps: []assets.WalkStep{}, Routes: map[byte][]assets.RoutePoint{1: {{Y: uint32(id)}}}},
		}, Events: []assets.Event{
			{ClickID: id, Branches: []assets.Branch{{Condition: [21]byte{byte(id)}, Operations: []assets.Operation{{Data: [21]byte{byte(id), 2}}, {Index: 1}}}}},
			{ClickID: id + 1, Branches: []assets.Branch{{Operations: []assets.Operation{}}, {Operations: nil}}},
		}, PreEvents: []assets.Event{}, Sections: [11][]byte{{byte(id)}, {}, nil}}
	}
	db := mapReadFixture(t, want)
	queries := map[string]int{}
	if err := db.Callback().Query().After("gorm:query").Register("test:catalog_query_bound", func(tx *gorm.DB) {
		queries[tx.Statement.Table]++
	}); err != nil {
		t.Fatal(err)
	}
	got, err := readMaps(db)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("batched map projection changed ownership/order/presence", got, err)
	}
	for table, count := range queries {
		if count != 1 {
			t.Errorf("%s read %d times; want one", table, count)
		}
	}
	rollback := errors.New("rollback")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := readMaps(tx); err != nil {
			return err
		}
		if err := tx.Model(&MapsRow{}).Where("maps_key = ?", 10).Update("value_scene", 99).Error; err != nil {
			return err
		}
		changed, err := readMaps(tx)
		if err == nil && changed[10].Scene != 99 {
			t.Error("read reused a stale projection after a SQL write")
		}
		if err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	got, err = readMaps(db)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("projection escaped rollback", err)
	}
}

func TestCatalogBatchedReadsRetainFailuresAndLazyChildren(t *testing.T) {
	for _, name := range []string{"ordinal", "byte_length", "query", "empty", "cancel"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := assets.Map{Events: []assets.Event{{Branches: []assets.Branch{{Operations: []assets.Operation{{}}}}}}}
			if name == "empty" {
				m = assets.Map{}
			}
			db := mapReadFixture(t, map[uint16]assets.Map{10: m})
			switch name {
			case "ordinal":
				if err := db.Model(&MapsEventsBranchesOperationsRow{}).Where("maps_key = ?", 10).Update("maps_events_branches_operations_ordinal", 2).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := readMaps(db); err == nil || !strings.Contains(err.Error(), "invalid catalog ordinal") {
					t.Fatal("invalid operation ordinal accepted", err)
				}
			case "byte_length":
				if err := db.Model(&MapsEventsBranchesRow{}).Where("maps_key = ?", 10).Update("value_condition", []byte{1}).Error; err != nil {
					t.Fatal(err)
				}
				if _, err := readMaps(db); err == nil || !strings.Contains(err.Error(), "byte length") {
					t.Fatal("invalid condition length accepted", err)
				}
			case "query":
				failure := errors.New("child query failed")
				if err := db.Callback().Query().Before("gorm:query").Register("test:catalog_query_failure", func(tx *gorm.DB) {
					if tx.Statement.Table == "catalog_maps_events" {
						tx.AddError(failure)
					}
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := readMaps(db); !errors.Is(err, failure) {
					t.Fatal("query failure swallowed", err)
				}
			case "empty":
				var tables []string
				if err := db.Callback().Query().After("gorm:query").Register("test:catalog_lazy", func(tx *gorm.DB) { tables = append(tables, tx.Statement.Table) }); err != nil {
					t.Fatal(err)
				}
				if _, err := readMaps(db); err != nil || !reflect.DeepEqual(tables, []string{"catalog_maps"}) {
					t.Fatal("absent children were fetched", tables, err)
				}
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, err := readMaps(db.WithContext(ctx)); !errors.Is(err, context.Canceled) {
					t.Fatal("caller cancellation lost", err)
				}
			}
		})
	}
}

func TestCatalogBatchedStringParentKeys(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "effects.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	for _, model := range catalogTables() {
		name := model.(interface{ TableName() string }).TableName()
		if name == "catalog_skill_effects" || strings.HasPrefix(name, "catalog_skill_effects_") {
			if err := db.Migrator().CreateTable(model); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := map[string]assets.EffectDefinition{
		"first":  {Effects: []assets.SkillEffect{{Modifiers: []assets.StatModifier{{Stat: assets.EffectATK, Flat: 7}}}}},
		"second": {Effects: []assets.SkillEffect{{Modifiers: []assets.StatModifier{{Stat: assets.EffectDEF, Flat: 9}}}}},
	}
	if err := writeSkillEffects(db, want); err != nil {
		t.Fatal(err)
	}
	got, err := readSkillEffects(db)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("string-keyed child ownership changed", got, err)
	}
}
