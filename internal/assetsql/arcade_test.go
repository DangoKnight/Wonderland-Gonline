package assetsql

import (
	"encoding/json"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func TestArcadeDefaultsNativeTablesAndEqualWeights(t *testing.T) {
	var seed []assets.ArcadeGame
	if err := json.Unmarshal(arcadeDefaults, &seed); err != nil {
		t.Fatal(err)
	}
	items := map[uint16]game.ItemDefinition{}
	for _, g := range seed {
		for _, r := range g.Rewards {
			items[r.ItemID] = game.ItemDefinition{ID: r.ItemID}
		}
	}
	got, err := defaultArcades(items)
	if err != nil || len(got) != 5 {
		t.Fatal(got, err)
	}
	for _, g := range got {
		expected := int64(11)
		if g.Kind == 6 || g.Kind == 22 {
			expected = 10
		}
		if !g.Enabled || g.TotalWeight() != expected {
			t.Fatal(g)
		}
		for roll := int64(0); roll < g.TotalWeight(); roll++ {
			reward, ok := g.RewardForRoll(roll)
			if !ok || reward.Weight != 1 || reward.Quantity != 1 || reward.ItemID != assets.ArcadePrizeItem(g.Kind, reward.Index) {
				t.Fatal(g, reward)
			}
		}
		if _, ok := g.RewardForRoll(g.TotalWeight()); ok {
			t.Fatal("out-of-range roll accepted")
		}
	}
	disabled, err := defaultArcades(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range disabled {
		if g.Enabled {
			t.Fatal("missing item machine enabled")
		}
	}
}

func TestArcadeSchemaV3UpgradeAndAdministrationPreserveEdits(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	// Reconstruct the exact v3 boundary: all old typed rows survive unchanged.
	if err = db.Migrator().DropTable(&ArcadesRewardsRow{}, &ArcadesRow{}, &TerrainsRow{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropColumn(&catalogPresence{}, "Terrains"); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropColumn(&catalogPresence{}, "Arcades"); err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 3).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&NativeItemsRow{}).Where("native_items_key = ?", 10002).Update("value_definition_name", "Existing v3 edit").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatabase(path)
	if err != nil || len(got.Arcades) != 5 || got.Items[10002].Name != "Existing v3 edit" {
		t.Fatal(got, err)
	}
	// This fixture lacks real prizes, so machines stay disabled. Weights remain
	// editable without enabling an invalid native item table.
	got.Arcades[0].Rewards[0].Weight = 37
	raw, err := json.Marshal(got.Arcades)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error {
		candidate, err := DefinitionCandidate(tx, "Arcades", raw, nil)
		if err != nil {
			return err
		}
		return SaveEditedCatalog(tx, candidate)
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(got.Arcades, reloaded.Arcades) || reloaded.Items[10002].Name != "Existing v3 edit" {
		t.Fatal("SQL edit overwritten", err)
	}
	got.Arcades[0].Rewards[0].Weight = -1
	invalid, _ := json.Marshal(got.Arcades)
	if err = db.Transaction(func(tx *gorm.DB) error { _, err := DefinitionCandidate(tx, "Arcades", invalid, nil); return err }); err == nil {
		t.Fatal("negative weight accepted")
	}
	got.Arcades[0].Rewards[0].Weight = 1
	got.Arcades[0].Rewards[0].ItemID = 10002
	invalid, _ = json.Marshal(got.Arcades)
	if err = db.Transaction(func(tx *gorm.DB) error { _, err := DefinitionCandidate(tx, "Arcades", invalid, nil); return err }); err == nil {
		t.Fatal("native prize identity mismatch accepted")
	}
}
