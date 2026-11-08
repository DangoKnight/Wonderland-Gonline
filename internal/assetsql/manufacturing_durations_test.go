package assetsql

import (
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/assetdb"
)

func TestDefaultManufactureDurations(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "assets.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { assetdb.Close(db) })
	if err := db.AutoMigrate(&assetdb.Record{}); err != nil {
		t.Fatal(err)
	}
	rows := []assetdb.Record{
		{Asset: "compound2.dat", Collection: "records", Ordinal: 0, JSON: `{"record_index":511,"fields":{"build_time":3}}`},
		{Asset: "compound2.dat", Collection: "records", Ordinal: 1, JSON: `{"record_index":42,"fields":{"build_time":0}}`},
		{Asset: "other.dat", Collection: "records", Ordinal: 0, JSON: `{"record_index":9,"fields":{"build_time":5}}`},
	}
	for _, row := range rows {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, err := DefaultManufactureDurations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[511] != 180 || got[42] != 0 {
		t.Fatalf("durations %v", got)
	}
	if _, ok := got[9]; ok {
		t.Fatal("foreign asset leaked into the durations")
	}
}

func TestDefaultManufactureDurationsWithoutRecords(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "assets.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { assetdb.Close(db) })
	got, err := DefaultManufactureDurations(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("durations %v", got)
	}
}
