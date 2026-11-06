package server

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/dbmigration"
	"wonderland-gonline/internal/world"
)

func TestStorySQLPublicationAndReload(t *testing.T) {
	source := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if source == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
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
	ctx := context.Background()
	current, err := s.ReadAssetDocument(ctx, "Story")
	if err != nil {
		t.Fatal(err)
	}
	var defs assets.StoryDefinitions
	if err := json.Unmarshal(current.Value, &defs); err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range defs.Rules {
		if defs.Rules[i].ID == "movie_12465" {
			defs.Rules[i].Value = 2
			found = true
		}
	}
	if !found {
		t.Fatal("movie policy absent")
	}
	raw, _ := json.Marshal(defs)
	edit := AssetEdit{Version: current.Version, Value: raw}
	players[0].event = &eventSession{}
	if err := s.EditAsset(ctx, "Story", edit, "", 0); err == nil {
		t.Fatal("active interaction allowed story replacement")
	}
	players[0].event = nil
	before := s.AssetSnapshot()
	if err := s.EditAsset(ctx, "Story", edit, "", 0); err != nil {
		t.Fatal(err)
	}
	x := world.StoryContext{Map: 12465, Event: 2}
	rule, ok := s.World.StoryFirst(assets.StoryMovieMode, nil, nil, x)
	if !ok || rule.Value != 2 {
		t.Fatal("typed edit did not replace the compiled policy index")
	}
	for _, r := range before.Story.Rules {
		if r.ID == "movie_12465" && r.Value != 1 {
			t.Fatal("edit mutated the published old snapshot")
		}
	}
	if err := s.EditAsset(ctx, "Story", edit, "", 0); !errors.Is(err, assetdb.ErrEditConflict) {
		t.Fatal("stale story edit accepted", err)
	}
	for i := range defs.Rules {
		if defs.Rules[i].ID == "movie_12465" {
			defs.Rules[i].Value = 1
		}
	}
	raw, _ = json.Marshal(defs)
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		c, err := assetsql.DefinitionCandidate(tx, "Story", raw, nil)
		if err != nil {
			return err
		}
		return assetsql.SaveEditedCatalog(tx, c)
	})
	assetdb.Close(db)
	if err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	err = s.gmReload(ctx, players[0], "reload", []string{"quests"})
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	rule, ok = s.World.StoryFirst(assets.StoryMovieMode, nil, nil, x)
	if !ok || rule.Value != 1 {
		t.Fatal("quest reload retained stale story policies")
	}
}
