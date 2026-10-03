package world

import (
	"os"
	"path/filepath"
	"testing"
	"wonderland-go/client/wlo/login"
)

var assets = login.NewAssets(filepath.Join("..", "..", "..", "data"))

func needAssets(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(assets.DataPath("ground_data.json")); err != nil {
		t.Skip("decompiled repository data not present")
	}
}

// TestShipDeck: map 10017 is scene 10001 "Ship Deck", a 2432×1792 scene
// with one background picture.
func TestShipDeck(t *testing.T) {
	needAssets(t)
	names, err := SceneNames(assets)
	if err != nil {
		t.Fatal(err)
	}
	maps, err := MapScenes(assets)
	if err != nil {
		t.Fatal(err)
	}
	if scene := maps[10017]; scene != 10001 || names[scene] != "Ship Deck" {
		t.Fatalf("scene %d %q", scene, names[scene])
	}
	s, err := LoadScene(assets, 10017)
	if err != nil {
		t.Fatal(err)
	}
	if s.Width != 2432 || len(s.Layers) != 1 || s.Layers[0].Image.W != 2432 || s.Layers[0].Image.H != 1792 {
		t.Fatalf("%dx%d, %d layers", s.Width, s.Height, len(s.Layers))
	}
}
