package assetsql

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"testing"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
)

func TestAdminSQLCatalogEditValidatesBeforeCommit(t *testing.T) {
	path := runtimeDatabaseFixture(t)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	ctx := context.Background()
	if err := assetdb.EnsureDocument(ctx, db, assets.AdminChestAsset); err != nil {
		t.Fatal(err)
	}
	before, err := assetdb.ReadDocument(db, assets.AdminChestAsset)
	if err != nil {
		t.Fatal(err)
	}
	validate := func(tx *gorm.DB) error { _, err := LoadTransaction(tx); return err }
	bad := []byte(`{"schema_version":1,"value":[{"map_id":10017,"respawn_seconds":60,"rewards":[{"item_id":65000,"count":1,"weight":1}]}]}`)
	if err := assetdb.ReplaceDocument(ctx, db, assets.AdminChestAsset, assetdb.DocumentVersion(before), bad, validate); err == nil {
		t.Fatal("unknown reward committed")
	}
	good := []byte(`{"schema_version":1,"value":[{"map_id":10017,"respawn_seconds":60,"rewards":[{"item_id":10002,"count":2,"weight":1}]}]}`)
	if err := assetdb.ReplaceDocument(ctx, db, assets.AdminChestAsset, assetdb.DocumentVersion(before), good, validate); err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadDatabase(path)
	if err != nil || len(catalog.ChestPools) != 1 {
		t.Fatal(err)
	}
	if err := assetdb.EnsureDocument(ctx, db, assets.AdminMapsAsset); err != nil {
		t.Fatal(err)
	}
	before, err = assetdb.ReadDocument(db, assets.AdminMapsAsset)
	if err != nil {
		t.Fatal(err)
	}
	m := catalog.Maps[10017]
	m.Warps = []assets.Warp{{ClickID: 1, MapID: 10017, X: 42, Y: 43}}
	raw, err := json.Marshal(map[string]any{"schema_version": 1, "value": []assets.Map{m}})
	if err != nil {
		t.Fatal(err)
	}
	if err := assetdb.ReplaceDocument(ctx, db, assets.AdminMapsAsset, assetdb.DocumentVersion(before), raw, validate); err != nil {
		t.Fatal(err)
	}
	catalog, err = LoadDatabase(path)
	if err != nil || len(catalog.Maps[10017].Warps) != 1 || catalog.Maps[10017].Warps[0].X != 42 {
		t.Fatal(catalog, err)
	}
}
