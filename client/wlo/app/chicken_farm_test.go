package app

import (
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/clientfs"
)

func TestChickenFarmTilePictures(t *testing.T) {
	var c *Client
	if bundle := os.Getenv("CHICKEN_FARM_TEST_BUNDLE"); bundle != "" {
		root, closeBundle, err := clientfs.Mount(bundle)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = closeBundle() })
		c, err = New(Options{Root: root, SettingsPath: filepath.Join(t.TempDir(), "settings.json"), ServerINI: filepath.Join(t.TempDir(), "SERVER.INI"), FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Sprites.Close() })
	} else {
		c = testClient(t)
	}
	c.dispatch(selfPacket(10001, 2, 11083, 1388, 965, 0, 444444444, 444444444, nil, "Dango"))
	if c.World == nil || len(c.World.Scene.Tiles) == 0 {
		t.Fatal("Chicken Farm terrain walls missing")
	}
	c.Frame()
	if path := os.Getenv("CHICKEN_FARM_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}
