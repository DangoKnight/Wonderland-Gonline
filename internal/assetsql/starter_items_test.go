package assetsql

import (
	"path/filepath"
	"reflect"
	"testing"

	"wonderland-gonline/internal/assetdb"
)

func TestStarterNotepadMigrationPreservesOtherGrants(t *testing.T) {
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "assets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&catalogSchema{}, &StarterItemsRow{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&catalogSchema{ID: catalogMetadataID, Version: 65}).Error; err != nil {
		t.Fatal(err)
	}
	want := []StarterItemsRow{{StarterItemsOrdinal: 0, ValueID: 34058, ValueCount: 1}, {StarterItemsOrdinal: 1, ValueID: 32176, ValueCount: 37}}
	rows := []StarterItemsRow{{StarterItemsOrdinal: 0, ValueID: 34038, ValueCount: 1}, {StarterItemsOrdinal: 1, ValueID: 34058, ValueCount: 1}, {StarterItemsOrdinal: 2, ValueID: 32176, ValueCount: 37}}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := db.Transaction(migrateCatalog); err != nil {
			t.Fatal(err)
		}
		var got []StarterItemsRow
		if err := db.Order("starter_items_ordinal").Find(&got).Error; err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("migration altered other grants: %+v", got)
		}
	}
}
