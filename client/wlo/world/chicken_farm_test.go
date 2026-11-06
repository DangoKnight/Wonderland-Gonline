package world

import (
	"testing"
	"wonderland-gonline/internal/clientruntime"
)

func TestChickenFarmObjects(t *testing.T) {
	needAssets(t)
	s, err := LoadScene(assets, 11083)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Objects) != 35 || len(s.Tiles) != 31 {
		t.Fatalf("missing native scenery: objects=%d tiles=%d", len(s.Objects), len(s.Tiles))
	}
	for _, tile := range s.Tiles {
		if tile.Image == nil || !tile.Sorted {
			t.Fatal("cliff tile missing image or depth ordering")
		}
	}
	shadows := 0
	for _, object := range s.Objects {
		if len(object.shadowFrames) > 0 {
			shadows++
		}
	}
	if shadows == 0 {
		t.Fatal("Chicken Farm's stippled tree shadows were not reconstructed")
	}
}

func TestNativeTerrainTileCoordinatesAndSignedDepth(t *testing.T) {
	needAssets(t)
	s := &Scene{MapID: 11083}
	err := s.loadTerrainTiles(assets, &clientruntime.TileGrid{Mode: 1, Rows: 1, Columns: 2, Cells: []clientruntime.TileCell{
		{Wall: 20000, WallVariant: 4, WallStyle: 1, Depth: 0xffffffbd},
		{Wall: 20000, WallVariant: 4, WallStyle: 1, Depth: 883},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tiles) != 2 || s.Tiles[0].X != 181 || s.Tiles[0].Y != -143 || s.Tiles[0].Depth != -67 || s.Tiles[1].X != 1 || s.Tiles[1].Y != -53 || s.Tiles[1].Depth != 883 {
		t.Fatal("native tile projection or signed sort line differs", s.Tiles)
	}
	if s.Tiles[0].Image != s.Tiles[1].Image {
		t.Fatal("identical tile pictures were decoded twice")
	}
	if err = s.loadTerrainTiles(assets, &clientruntime.TileGrid{Mode: 1, Rows: 39, Columns: 1}); err == nil {
		t.Fatal("oversized native grid accepted")
	}
}
