package assetsql

import (
	"os"
	"testing"
	"wonderland-go/internal/world"
)

func TestInstalledSQLWorldSimulationGeometry(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Terrains) != 1147 {
		t.Fatal("native terrain census", len(c.Terrains))
	}
	terrain := c.Terrains[10017]
	if terrain.Width != 2432 || terrain.Height != 1792 || terrain.GridWidth != 122 || terrain.GridHeight != 90 {
		t.Fatal("Ship Deck dimensions", terrain.Width, terrain.Height)
	}
	w := world.New(c)
	if !w.CanMove(10017, 1042, 1075, 1122, 1075) || w.CanMove(10017, 1042, 1075, 0, 0) || w.Contains(10017, 2432, 1000) {
		t.Fatal("Ship Deck walk/collision rules")
	}
}
