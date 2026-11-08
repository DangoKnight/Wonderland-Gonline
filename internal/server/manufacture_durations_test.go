package server

import (
	"context"
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/game"

	"gorm.io/gorm"
)

func manufactureDurationDB(t *testing.T, durations map[uint16]uint32) *gorm.DB {
	t.Helper()
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "assets.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { assetdb.Close(db) })
	if err := db.AutoMigrate(&assetsql.ManufacturingRow{}); err != nil {
		t.Fatal(err)
	}
	for id, duration := range durations {
		row := assetsql.ManufacturingRow{ManufacturingKey: id, ValueDurationSeconds: duration}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func readDuration(t *testing.T, db *gorm.DB, id uint16) uint32 {
	t.Helper()
	var row assetsql.ManufacturingRow
	if err := db.Where("manufacturing_key = ?", id).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	return row.ValueDurationSeconds
}

func TestApplyManufactureDurations(t *testing.T) {
	db := manufactureDurationDB(t, map[uint16]uint32{511: 60, 499: 0, 7: 30})
	updated, err := applyManufactureDurations(db, []uint16{511}, false, 5)
	if err != nil || updated != 1 {
		t.Fatal(updated, err)
	}
	if readDuration(t, db, 511) != 5 || readDuration(t, db, 499) != 0 || readDuration(t, db, 7) != 30 {
		t.Fatal("single formula update touched other rows")
	}
	updated, err = applyManufactureDurations(db, nil, true, 0)
	if err != nil || updated != 3 {
		t.Fatal(updated, err)
	}
	for _, id := range []uint16{511, 499, 7} {
		if readDuration(t, db, id) != 0 {
			t.Fatalf("formula %d kept its duration", id)
		}
	}
}

func TestApplyManufactureDefaults(t *testing.T) {
	db := manufactureDurationDB(t, map[uint16]uint32{511: 0, 499: 0, 7: 0})
	var updated int64
	err := db.Transaction(func(tx *gorm.DB) error {
		got, e := applyManufactureDefaults(tx, map[uint16]uint32{511: 180, 499: 60})
		updated = got
		return e
	})
	if err != nil || updated != 2 {
		t.Fatal(updated, err)
	}
	if readDuration(t, db, 511) != 180 || readDuration(t, db, 499) != 60 || readDuration(t, db, 7) != 0 {
		t.Fatal("defaults not restored")
	}
}

func TestSetManufactureDurationsValidatesRange(t *testing.T) {
	s, _, _ := tentFixture(t)
	if _, err := s.SetManufactureDurations(context.Background(), []uint16{1}, false, game.MaxGold); err == nil {
		t.Fatal("oversized duration accepted")
	}
}
