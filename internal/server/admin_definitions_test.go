package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/dbmigration"
	"wonderland-gonline/internal/world"
)

func TestAdminDefinitionsVersionRollbackAndInteractionGates(t *testing.T) {
	source := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if source == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join("..", "..", source)
	}
	path := filepath.Join(t.TempDir(), "assets.db")
	if err := dbmigration.Copy(source, path, true); err != nil {
		t.Fatal(err)
	}
	catalog, err := assetsql.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	s, players, _ := chatFixture(t)
	s.Config.AssetsDatabase = path
	s.Assets = catalog
	s.World = world.New(catalog)
	var id uint16
	for key := range catalog.NPCs {
		id = key
		break
	}
	ctx := context.Background()
	records, err := s.ReadAssetRecords(ctx, "NPCs", int64(id))
	if err != nil || len(records) != 1 {
		t.Fatal(records, err)
	}
	npc := catalog.NPCs[id]
	npc.Name = "Typed administration edit"
	raw, _ := json.Marshal(npc)
	edit := AssetEdit{Version: records[0].Version, Value: raw}
	players[0].event = &eventSession{}
	if err = s.EditAsset(ctx, "NPCs", edit, "NPCs", int(id)); err == nil {
		t.Fatal("active event bypassed")
	}
	players[0].event = nil
	if err = s.EditAsset(ctx, "NPCs", edit, "NPCs", int(id)); err != nil {
		t.Fatal(err)
	}
	if s.Assets.NPCs[id].Name != npc.Name {
		t.Fatal("snapshot was not published")
	}
	if err = s.EditAsset(ctx, "NPCs", edit, "NPCs", int(id)); !errors.Is(err, assetdb.ErrEditConflict) {
		t.Fatal("stale edit accepted", err)
	}
	current, err := s.ReadAssetRecords(ctx, "NPCs", int64(id))
	if err != nil {
		t.Fatal(err)
	}
	writer, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(writer)
	if err = writer.Exec("CREATE TRIGGER fail_definition BEFORE INSERT ON catalog_native_items BEGIN SELECT RAISE(ABORT,'injected save failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	npc.Name = "Must roll back"
	raw, _ = json.Marshal(npc)
	if err = s.EditAsset(ctx, "NPCs", AssetEdit{Version: current[0].Version, Value: raw}, "NPCs", int(id)); err == nil {
		t.Fatal("failed save accepted")
	}
	after, err := s.ReadAssetRecords(ctx, "NPCs", int64(id))
	if err != nil || after[0].Version != current[0].Version || s.Assets.NPCs[id].Name == npc.Name {
		t.Fatal("partial database or snapshot publication", err)
	}
}
