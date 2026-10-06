package assetsql

import (
	"encoding/json"
	"gorm.io/gorm"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestFishingSchemaV6UpgradeAndPreservedEdits(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	for _, model := range []any{&FishingSkillsRow{}, &FishingCatchRequirementsRow{}, &FishingRodsRow{}, &FishingMapsRow{}, &FishingRewardsRow{}, &FishingRow{}} {
		if err = db.Migrator().DropTable(model); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 6).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Fishing.Enabled || c.Fishing.IntervalSeconds != 60 || len(c.Fishing.Rods) != 3 || len(c.Fishing.Rewards) == 0 {
		t.Fatal("missing definitions should disable the seeded rules", c.Fishing)
	}
	c.Fishing.IntervalSeconds = 120
	if err = db.Transaction(func(tx *gorm.DB) error { return SaveEditedCatalog(tx, c) }); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	next, err := LoadDatabase(path)
	if err != nil || !reflect.DeepEqual(c.Fishing, next.Fishing) {
		t.Fatal("migration replaced fishing edits", err)
	}
}

// Check the resulting category share rather than duplicating the seed weights.
func TestFishingDefaultFishShare(t *testing.T) {
	var f assets.FishingRules
	if err := json.Unmarshal(fishingDefaults, &f); err != nil {
		t.Fatal(err)
	}
	fish := map[uint16]bool{41091: true, 41092: true, 41094: true, 41096: true, 41097: true, 41098: true, 41100: true, 41101: true}
	for _, rod := range f.Rods {
		var total, fishWeight uint64
		for _, reward := range f.Rewards {
			weight := f.Weight(reward, rod, 0)
			total += weight
			if fish[reward.ItemID] {
				fishWeight += weight
			}
		}
		share := float64(fishWeight) / float64(total)
		if share < 0.55 || share > 0.56 {
			t.Fatalf("rod %d fish share %.4f", rod.ItemID, share)
		}
	}
}

func TestFishingSchemaV7WeightsUpgradePreservesCustomization(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	f, err := readFishing(db)
	if err != nil {
		t.Fatal(err)
	}
	var previous []assets.FishingReward
	if err = json.Unmarshal(fishingV7Weights, &previous); err != nil {
		t.Fatal(err)
	}
	oldWeights := map[uint16]uint32{}
	for _, old := range previous {
		oldWeights[old.ItemID] = old.Weight
	}
	expected := f
	expected.IntervalSeconds = 120
	expected.Rewards = append([]assets.FishingReward(nil), f.Rewards...)
	expected.Rewards[0].Weight = 7777 // Custom weight.
	expected.Rewards[1].Grade = 3     // Custom grade with the old weight.
	expected.Rewards[1].Weight = oldWeights[expected.Rewards[1].ItemID]
	// A removed catch must not be reintroduced by the migration.
	expected.Rewards = append(expected.Rewards[:4], expected.Rewards[5:]...)
	f = expected
	f.Rewards = append([]assets.FishingReward(nil), expected.Rewards...)
	for i := range f.Rewards {
		if old, ok := oldWeights[f.Rewards[i].ItemID]; ok && i != 0 {
			f.Rewards[i].Weight = old
		}
	}
	if err = db.Transaction(func(tx *gorm.DB) error {
		if err := replaceFishingForTest(tx, f); err != nil {
			return err
		}
		return tx.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 7).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err := readFishing(db)
	if err != nil || !reflect.DeepEqual(got, expected) {
		t.Fatalf("customizations or reward order changed: %v\n%+v\n%+v", err, got, expected)
	}
	// Subsequent administrator edits must also survive repeated migration.
	got.Rewards[2].Weight = 999
	if err = db.Transaction(func(tx *gorm.DB) error { return replaceFishingForTest(tx, got) }); err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	again, err := readFishing(db)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("repeated migration replaced custom weights", err)
	}
}

func TestFishingWeightsUpgradeRollsBack(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	if err := MigrateDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	var previous []assets.FishingReward
	if err = json.Unmarshal(fishingV7Weights, &previous); err != nil {
		t.Fatal(err)
	}
	for _, old := range previous {
		if err = db.Model(&FishingRewardsRow{}).Where("value_item_id = ?", old.ItemID).Update("value_weight", old.Weight).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", 7).Error; err != nil {
		t.Fatal(err)
	}
	before, err := readFishing(db)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("CREATE TRIGGER fail_fishing_weight_upgrade BEFORE UPDATE ON catalog_fishing_rewards WHEN OLD.value_item_id = 41101 BEGIN SELECT RAISE(ABORT, 'forced weight upgrade failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(migrateCatalog); err == nil {
		t.Fatal("failed upgrade accepted")
	}
	after, err := readFishing(db)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("failed upgrade changed fishing weights", err)
	}
	var schema catalogSchema
	if err = db.First(&schema, catalogMetadataID).Error; err != nil || schema.Version != 7 {
		t.Fatal("failed upgrade advanced schema", schema, err)
	}
}

func replaceFishingForTest(tx *gorm.DB, f assets.FishingRules) error {
	if err := tx.Where("fishing_key = ?", catalogMetadataID).Delete(&FishingRow{}).Error; err != nil {
		return err
	}
	return writeFishing(tx, f)
}
