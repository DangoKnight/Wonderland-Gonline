package world

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientruntime"
)

func TestChickenFarmNorthernTreeCoordinates(t *testing.T) {
	needAssets(t)
	scene, err := LoadScene(assets, 11083)
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("CHICKEN_FARM_NORTH_SNAPSHOT"); path != "" {
		screen := surface.New(800, 600)
		w := &World{Scene: scene, Env: &seui.Env{Screen: screen, Pics: picdb.New()},
			Now: func() time.Time { return time.Unix(100, 0) }, HideNames: true, Cinematic: true,
			CameraAt: &image.Point{X: 782}, DeferOverlays: true}
		w.Draw()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, screen.RGBA()); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Authored Ground.MMG placements crossing the map's upper/left borders.
	// The reference viewport is centered at player X1182, Y295.
	for _, want := range []struct {
		resource uint32
		x, y     int
	}{
		{11022026, 811, -91}, {11022024, 988, -70},
		{11022025, -80, 435}, {11022025, 1942, -161}, {11022026, 2224, -86},
	} {
		found := false
		for _, object := range scene.Objects {
			if object.Resource == want.resource && object.X == want.x && object.Y == want.y {
				found = true
				if object.SortY() != want.y+object.Depth {
					t.Fatal("negative placement did not reach the signed depth ordering")
				}
				break
			}
		}
		if !found {
			t.Errorf("tree %d missing at native signed position %d,%d", want.resource, want.x, want.y)
		}
	}
}

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
