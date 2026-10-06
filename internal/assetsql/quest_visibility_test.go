package assetsql

import (
	"context"
	"gorm.io/gorm"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestQuestVisibilitySQLProjectionAndRollback(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	ctx := context.Background()
	if err := assetdb.EnsureDocument(ctx, db, assets.QuestVisibilityAsset); err != nil {
		t.Fatal(err)
	}
	before, err := assetdb.ReadDocument(db, assets.QuestVisibilityAsset)
	if err != nil {
		t.Fatal(err)
	}
	validate := func(tx *gorm.DB) error { _, err := loadLegacyTransaction(tx); return err }
	valid := []byte(`{"schema_version":1,"value":[{"quest_id":9,"map_id":10017}]}`)
	if err := assetdb.ReplaceDocument(ctx, db, assets.QuestVisibilityAsset, assetdb.DocumentVersion(before), valid, validate); err != nil {
		t.Fatal(err)
	}
	catalog, err := loadLegacyFixture(path)
	if err != nil || len(catalog.QuestVisibility) != 1 || catalog.QuestVisibility[0].ID != 9 {
		t.Fatal(catalog, err)
	}
	before, err = assetdb.ReadDocument(db, assets.QuestVisibilityAsset)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []byte(`{"schema_version":1,"value":[{"quest_id":9,"map_id":10017,"spawn_npc_click_ids":[999]}]}`)
	if err := assetdb.ReplaceDocument(ctx, db, assets.QuestVisibilityAsset, assetdb.DocumentVersion(before), invalid, validate); err == nil {
		t.Fatal("invalid actor committed")
	}
	catalog, err = loadLegacyFixture(path)
	if err != nil || len(catalog.QuestVisibility) != 1 || len(catalog.QuestVisibility[0].Spawn) != 0 {
		t.Fatal("invalid edit changed projection", err)
	}
	// Indexed rows, rather than document JSON, are authoritative.
	if err := db.Model(&assetdb.Record{}).Where("asset = ?", assets.QuestVisibilityAsset).Update("json", `{"quest_id":77,"map_id":10017}`).Error; err != nil {
		t.Fatal(err)
	}
	catalog, err = loadLegacyFixture(path)
	if err != nil || catalog.QuestVisibility[0].ID != 77 {
		t.Fatal("projection ignored indexed SQL", err)
	}
}
