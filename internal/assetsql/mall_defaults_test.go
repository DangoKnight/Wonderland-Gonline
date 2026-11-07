package assetsql

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestMallScreenshotDefaults(t *testing.T) {
	var entries []assets.MallItem
	if err := json.Unmarshal(mallScreenshotDefaults, &entries); err != nil {
		t.Fatal(err)
	}
	counts := [2][6]int{}
	hot := [2]int{}
	for _, e := range entries {
		category := e.CategoryID
		if category == 4 {
			category = 3
		}
		counts[e.Bonus][category]++
		if e.Badge != 0 {
			hot[e.Bonus]++
		}
	}
	// Independently counted screenshot pages, excluding duplicated Hot views.
	if counts != [2][6]int{{0, 10, 9, 130, 0, 10}, {0, 12, 15, 35, 0, 9}} || hot != [2]int{55, 16} {
		t.Fatal("screenshot page census", counts, hot)
	}
	for _, want := range []struct{ bonus, order, id, count, cost, original, discount int }{
		{0, 1, 57819, 1, 32, 32, 100}, {0, 13, 55086, 1, 20, 25, 80},
		{0, 28, 55073, 50, 50, 50, 100}, {0, 36, 34269, 5, 119, 119, 100},
		{0, 37, 34335, 1, 13, 14, 90}, {0, 92, 30124, 1, 1, 4, 30},
		{0, 109, 30099, 1, 36, 36, 100}, {0, 114, 25848, 1, 360, 360, 100},
		{1, 1, 30914, 1, 450, 450, 100}, {1, 27, 55073, 50, 3, 3, 100},
	} {
		found := false
		for _, e := range entries {
			if e.Bonus == want.bonus && e.Order == want.order {
				found = true
				if int(e.ID) != want.id || int(e.Count) != want.count || e.Cost != want.cost || e.OriginalPrice != want.original || e.Discount != want.discount {
					t.Fatalf("%+v: %+v", want, e)
				}
			}
		}
		if !found {
			t.Fatal("missing listing", want)
		}
	}
}

func TestMallAdditionalContainerDefaults(t *testing.T) {
	entries, err := defaultMall()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 251 {
		t.Fatal("mall census", len(entries))
	}
	wanted := map[uint16]int{34174: 35, 34193: 35, 34349: 54, 34365: 58, 34366: 58, 34385: 32, 34386: 32, 34387: 32, 34388: 32, 55811: 32, 56059: 32, 56239: 32, 56573: 32, 56619: 32, 57301: 32, 57884: 32, 57887: 32, 57921: 32, 35450: 132, 35451: 132, 56276: 32}
	for _, e := range entries {
		if cost, ok := wanted[e.ID]; ok {
			if e.Bonus != 0 || e.Cost != cost || e.New != 1 || e.Badge != 1 || e.Count != 1 {
				t.Fatal("additional container", e)
			}
			delete(wanted, e.ID)
		}
	}
	if len(wanted) != 0 {
		t.Fatal("missing containers", wanted)
	}
	var screenshot []assets.MallItem
	if err := json.Unmarshal(mallScreenshotDefaults, &screenshot); err != nil {
		t.Fatal(err)
	}
	updated, err := updateDefaultMall(screenshot)
	if err != nil || !reflect.DeepEqual(updated, entries) {
		t.Fatal("v59 update", err)
	}
	screenshot[0].Cost++
	updated, err = updateDefaultMall(screenshot)
	if err != nil || !reflect.DeepEqual(updated, screenshot) {
		t.Fatal("custom v59 catalog changed", err)
	}
}

func TestMallRepositoryItemDefinitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.db")
	if _, err := InitializeRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := defaultMall()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Mall, want) {
		t.Fatal("repository seed mall differs from authored JSON")
	}
	for _, e := range c.Mall {
		if _, ok := c.Items[e.ID]; !ok {
			t.Fatal("missing item definition", e.ID)
		}
		if !c.GachaAvailable(e.ID) && e.ID != incompleteWinterClothBagID {
			t.Fatal("disabled mall item", e.ID)
		}
	}
}

func TestMallMigrationPreservesCustomAndEmptyCatalog(t *testing.T) {
	var previous []assets.MallItem
	if err := json.Unmarshal(mallLegacyDefaults, &previous); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"default", "custom", "empty"} {
		t.Run(mode, func(t *testing.T) {
			db, err := assetdb.Open(filepath.Join(t.TempDir(), "mall.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer assetdb.Close(db)
			if err := db.AutoMigrate(&MallRow{}, &catalogSchema{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&catalogSchema{ID: catalogMetadataID, Version: 58}).Error; err != nil {
				t.Fatal(err)
			}
			before := append([]assets.MallItem{}, previous...)
			if mode == "custom" {
				before[0].Cost++
			}
			if mode == "empty" {
				before = []assets.MallItem{}
			}
			if err := writeMall(db, before); err != nil {
				t.Fatal(err)
			}
			// The content and schema revision share a transaction.
			if err := db.Exec("CREATE TRIGGER fail_mall BEFORE UPDATE OF version ON catalog_schema BEGIN SELECT RAISE(ABORT, 'forced mall failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Transaction(migrateCatalog); err == nil {
				t.Fatal("accepted failed schema commit")
			}
			rolled, err := readMall(db)
			if err != nil || !reflect.DeepEqual(rolled, before) {
				t.Fatal("rollback", err)
			}
			if err := db.Exec("DROP TRIGGER fail_mall").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Transaction(migrateCatalog); err != nil {
				t.Fatal(err)
			}
			after, err := readMall(db)
			if err != nil {
				t.Fatal(err)
			}
			want := before
			if mode == "custom" {
				// v61 intentionally refreshes promotional fields only; every
				// customized price, category, quantity and ordering must survive.
				want = append([]assets.MallItem{}, before...)
				for i := range want {
					want[i].Badge, want[i].Hot, want[i].New, want[i].Limited = after[i].Badge, after[i].Hot, after[i].New, after[i].Limited
				}
			}
			if mode == "default" {
				want, err = defaultMall()
				if err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(want, after) {
				t.Fatal("catalog not preserved/updated")
			}
			if err := db.Transaction(migrateCatalog); err != nil {
				t.Fatal(err)
			}
			again, err := readMall(db)
			if err != nil || !reflect.DeepEqual(again, want) {
				t.Fatal("not idempotent", err)
			}
		})
	}
}
