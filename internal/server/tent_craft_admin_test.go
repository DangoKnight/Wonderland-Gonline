package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/dbmigration"
	"wonderland-gonline/internal/world"
)

func TestAdministratorTentCraftDurationsOnCatalog(t *testing.T) {
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
	s, _, _ := chatFixture(t)
	s.Config.AssetsDatabase = path
	s.Assets = catalog
	s.World = world.New(catalog)
	ctx := context.Background()

	updated, err := s.SetManufactureDurations(ctx, nil, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if updated != int64(len(catalog.Manufacturing)) {
		t.Fatalf("updated %d of %d formulas", updated, len(catalog.Manufacturing))
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var remaining int64
	if err := db.Model(&assetsql.ManufacturingRow{}).Where("value_duration_seconds <> 0").Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	assetdb.Close(db)
	if remaining != 0 {
		t.Fatalf("%d formulas kept a duration", remaining)
	}
	if got := s.AssetSnapshot().Manufacturing[511].DurationSeconds; got != 0 {
		t.Fatalf("served duration %d", got)
	}

	if _, err := s.ResetManufactureDurations(ctx); err != nil {
		t.Fatal(err)
	}
	if got := s.AssetSnapshot().Manufacturing[511].DurationSeconds; got != 0 {
		t.Fatalf("kiln duration after reset %d", got)
	}
	for id, formula := range s.AssetSnapshot().Manufacturing {
		if formula.DurationSeconds > assets.ManufacturingMaxDurationSeconds {
			t.Fatalf("formula %d duration %d", id, formula.DurationSeconds)
		}
	}
}
