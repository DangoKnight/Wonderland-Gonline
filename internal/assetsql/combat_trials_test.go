package assetsql

import (
	"context"
	"gorm.io/gorm"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestCombatTrialsSQLValidationProjectionAndRollback(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	ctx := context.Background()
	if err := db.Model(&assetdb.Record{}).Where("asset = ?", "npc.dat").Update("json", `{"fields":{"id":42,"hp":100,"level":5},"name":{"text":"Guardian"}}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := assetdb.EnsureDocument(ctx, db, assets.CombatTrialsAsset); err != nil {
		t.Fatal(err)
	}
	before, err := assetdb.ReadDocument(db, assets.CombatTrialsAsset)
	if err != nil {
		t.Fatal(err)
	}
	validate := func(tx *gorm.DB) error { _, err := loadLegacyTransaction(tx); return err }
	valid := []byte(`{"schema_version":1,"value":[{"stage":1,"name":"Aries","map_id":10017,"guardian_id":42,"hp":15000,"attack":450,"reward_item_id":10002,"reward_count":1}]}`)
	if err := assetdb.ReplaceDocument(ctx, db, assets.CombatTrialsAsset, assetdb.DocumentVersion(before), valid, validate); err != nil {
		t.Fatal(err)
	}
	catalog, err := loadLegacyFixture(path)
	if err != nil || len(catalog.CombatTrials) != 1 || catalog.CombatTrials[0].HP != 15000 {
		t.Fatal("trial projection", err)
	}
	before, err = assetdb.ReadDocument(db, assets.CombatTrialsAsset)
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte(`{"schema_version":1,"value":[{"stage":1,"name":"Aries","map_id":10017,"guardian_id":1001,"hp":15000,"attack":450,"reward_item_id":10002,"reward_count":1}]}`)
	if err := assetdb.ReplaceDocument(ctx, db, assets.CombatTrialsAsset, assetdb.DocumentVersion(before), bad, validate); err == nil {
		t.Fatal("placeholder guardian accepted")
	}
	catalog, err = loadLegacyFixture(path)
	if err != nil || catalog.CombatTrials[0].Guardian != 42 {
		t.Fatal("failed edit changed trial", err)
	}
	// Indexed records supply runtime values, regardless of retained document JSON.
	if err := db.Model(&assetdb.Record{}).Where("asset = ?", assets.CombatTrialsAsset).Update("json", `{"stage":1,"name":"Aries","map_id":10017,"guardian_id":42,"hp":20000,"attack":450,"reward_item_id":10002,"reward_count":1}`).Error; err != nil {
		t.Fatal(err)
	}
	catalog, err = loadLegacyFixture(path)
	if err != nil || catalog.CombatTrials[0].HP != 20000 {
		t.Fatal("trial ignored indexed SQL", err)
	}
}
