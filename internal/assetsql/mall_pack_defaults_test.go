package assetsql

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestMallAdditionalPoolsAndPreservingMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.db")
	if _, err := InitializeRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	pool := c.GachaPacks[34193]
	want := []int{1600, 1300, 1100, 1000, 850, 750, 650, 550, 500, 400, 300, 300, 250, 200, 150, 40, 30, 20, 10}
	for i, r := range pool.Rewards {
		if r.Weight != want[i] {
			t.Fatal("published probabilities", pool)
		}
	}
	for _, id := range []uint16{35450, 35451} {
		p := c.GachaPacks[id]
		if !p.OpenAll || len(p.Rewards) != 3 || p.Rewards[2].ID != 22593 {
			t.Fatal("winter bundle", p)
		}
	}
	if c.GachaAvailable(incompleteWinterClothBagID) {
		t.Fatal("incomplete pool purchasable")
	}
	if _, err := assets.ParseGachaPacks(mallPackDefaults, c.Items); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint16{55811, 56059, 56239, 56573, 56619, 57301, 57884, 57887, 57921} {
		p := c.GachaPacks[id]
		total, minw, maxw := 0, 10000, 0
		for _, r := range p.Rewards {
			total += r.Weight
			minw = min(minw, r.Weight)
			maxw = max(maxw, r.Weight)
		}
		if total != 10000 || maxw-minw > 1 {
			t.Fatal("near-equal pool", p)
		}
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	var previous []assets.GachaPack
	if err := json.Unmarshal(mallPackPrevious, &previous); err != nil {
		t.Fatal(err)
	}
	old := previous[0]
	old.Rewards[0].Weight--
	old.Rewards[1].Weight++
	if err := db.Where("gacha_packs_key = ?", old.ID).Delete(&GachaPacksRewardsRow{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("gacha_packs_key = ?", old.ID).Delete(&GachaPacksRow{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeGachaPacks(db, map[uint16]assets.GachaPack{old.ID: old}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateMallPackDefaults); err != nil {
		t.Fatal(err)
	}
	got, err := readGachaPacks(db)
	if err != nil || !reflect.DeepEqual(got[old.ID], old) {
		t.Fatal("custom pool overwritten", err)
	}
	if err := db.Transaction(migrateMallPackDefaults); err != nil {
		t.Fatal(err)
	}
	again, err := readGachaPacks(db)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("non-idempotent pool migration", err)
	}
}
