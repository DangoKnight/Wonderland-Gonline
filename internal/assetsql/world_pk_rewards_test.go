package assetsql

import (
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestWorldPKRewardDefaultsAndSQLPreservation(t *testing.T) {
	defaults, err := defaultWorldPKRewards()
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults.Chests) != 3 {
		t.Fatal("missing chest tiers")
	}
	for i, expected := range []struct {
		tier                     string
		weight, low, high, count int
	}{
		{"wood", 85, 1, 3, 3}, {"silver", 10, 3, 5, 4}, {"gold", 5, 5, 9, 4},
	} {
		chest := defaults.Chests[i]
		if chest.Tier != expected.tier || chest.Weight != expected.weight || len(chest.Rewards) != expected.count || chest.Rewards[0].Minimum != expected.low || chest.Rewards[0].Maximum != expected.high {
			t.Fatalf("mobile chest mismatch: %+v", chest)
		}
		for _, reward := range chest.Rewards {
			if reward.Weight != 0 || reward.Unresolved == "" {
				t.Fatal("unverified reward enabled", reward)
			}
		}
	}
	if capsule := defaults.Chests[0].Rewards[1]; capsule.Minimum != 0 || capsule.Maximum != 0 || capsule.ValueMinimum != 500 || capsule.ValueMaximum != 1000 {
		t.Fatal("capsule denomination became quantity", capsule)
	}
	if crystal := defaults.Chests[2].Rewards[2]; crystal.ItemID != 61062 || crystal.Minimum != 3 || crystal.Maximum != 4 {
		t.Fatal("gold crystal mismatch", crystal)
	}
	db, err := assetdb.Open(filepath.Join(t.TempDir(), "rewards.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	if err := db.AutoMigrate(&catalogSchema{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&catalogSchema{ID: catalogMetadataID, Version: 57}).Error; err != nil {
		t.Fatal(err)
	}
	// Failure after table creation must roll back tables and schema together.
	if err := db.Exec("CREATE TRIGGER fail_world_pk BEFORE UPDATE OF version ON catalog_schema BEGIN SELECT RAISE(ABORT, 'forced reward failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err == nil {
		t.Fatal("migration failure accepted")
	}
	if db.Migrator().HasTable(&WorldPKRewardsRow{}) {
		t.Fatal("failed migration left reward tables")
	}
	if err := db.Exec("DROP TRIGGER fail_world_pk").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err := readWorldPKRewards(db)
	if err != nil || !reflect.DeepEqual(got, defaults) {
		t.Fatal("SQL reward round trip changed defaults", got, err)
	}
	got.Chests[0].Weight = 70
	if err := db.Model(&WorldPKRewardsChestsRow{}).Where("world_p_k_rewards_chests_ordinal = ?", 0).Update("value_weight", 70).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 57).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	loaded, err := readWorldPKRewards(db)
	if err != nil || !reflect.DeepEqual(loaded, got) {
		t.Fatal("migration overwrote administrator edits", err)
	}
	// Explicitly emptied definitions must not be repopulated by migration.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&WorldPKRewardsChestsRewardsRow{}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&WorldPKRewardsChestsRow{}).Error; err != nil {
			return err
		}
		return tx.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 57).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	loaded, err = readWorldPKRewards(db)
	if err != nil || len(loaded.Chests) != 0 {
		t.Fatal("empty pool reseeded", err)
	}
}

func TestWorldPKRewardValidation(t *testing.T) {
	for _, corrupt := range []func(*assets.WorldPKRewardDefinitions){
		func(v *assets.WorldPKRewardDefinitions) { v.Chests[1].Tier = v.Chests[0].Tier },
		func(v *assets.WorldPKRewardDefinitions) { v.Chests[0].Weight = -1 },
		func(v *assets.WorldPKRewardDefinitions) { v.Chests[0].Rewards[0].Maximum = 0 },
		func(v *assets.WorldPKRewardDefinitions) { v.Chests[0].Rewards[1].Unresolved = "" },
		func(v *assets.WorldPKRewardDefinitions) { v.Chests[0].Rewards[0].ItemID = 61062 },
	} {
		v, err := defaultWorldPKRewards()
		if err != nil {
			t.Fatal(err)
		}
		corrupt(&v)
		if assets.ValidateWorldPKRewards(v) == nil {
			t.Fatal("invalid reward definition accepted", v)
		}
	}
}
