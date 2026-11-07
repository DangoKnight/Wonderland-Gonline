package assetsql

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestMallFeaturedContainersPreserveCategoriesAndPrices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mall.db")
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&MallRow{}); err != nil {
		t.Fatal(err)
	}
	entries, err := defaultMall()
	if err != nil {
		t.Fatal(err)
	}
	var screenshot []assets.MallItem
	if err := json.Unmarshal(mallScreenshotDefaults, &screenshot); err != nil {
		t.Fatal(err)
	}
	// Reproduce v60's old promotional tags, with a customized item price.
	copy(entries, screenshot)
	entries[0].Cost = 99
	if err := writeMall(db, entries); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateMallFeaturedContainers); err != nil {
		t.Fatal(err)
	}
	got, err := readMall(db)
	if err != nil {
		t.Fatal(err)
	}
	featured := 0
	for i, e := range got {
		if i < len(screenshot) {
			if e.Badge != 0 || e.New != 0 || e.Hot != 0 || e.Limited != 0 {
				t.Fatal("old item still featured", e)
			}
		} else {
			if e.Badge != 1 || e.New != 1 {
				t.Fatal("container missing from Hot", e)
			}
			featured++
		}
		before := entries[i]
		before.Badge, before.New, before.Hot, before.Limited = e.Badge, e.New, e.Hot, e.Limited
		if !reflect.DeepEqual(before, e) {
			t.Fatal("non-promotional data changed", before, e)
		}
	}
	if featured != 21 || got[0].Cost != 99 {
		t.Fatal("featured count or custom price", featured)
	}
	if err := db.Transaction(migrateMallFeaturedContainers); err != nil {
		t.Fatal(err)
	}
	again, err := readMall(db)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("repeat migration", err)
	}
}
