package world

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/client/wlo/login"
)

var assets = login.NewAssets(filepath.Join("..", "..", "..", "data"))

func needAssets(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(assets.DataPath("ground_data.json")); err != nil {
		t.Skip("decompiled repository data not present")
	}
}

// TestShipDeck: map 10017 is scene 10001 (IGG English "Deck"), a 2432×1792 scene
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
	if scene := maps[10017]; scene != 10001 || names[scene] != "Deck" {
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

// TestCameraStaysOnMap follows FUN_003f94e8 on Dango Beach (1664 × 1280):
// centred in the middle, 0 near the left and top, width − 820 near the
// right and height − 600 near the bottom (Chat_02/Beach_Camera_Clamped.png
// at X:1082 Y:1195).
func TestCameraStaysOnMap(t *testing.T) {
	w := &World{Scene: &Scene{Width: 1664, Height: 1280}}
	for _, c := range []struct{ x, y, cx, cy int }{
		{1082, 1195, 682, 680},
		{800, 600, 400, 300},
		{300, 200, 0, 0},
		{1600, 1270, 1664 - 820, 680},
	} {
		w.Player.X, w.Player.Y = c.x, c.y
		if cx, cy := w.Camera(); cx != c.cx || cy != c.cy {
			t.Errorf("player (%d, %d): camera (%d, %d), want (%d, %d)", c.x, c.y, cx, cy, c.cx, c.cy)
		}
	}
}

func TestCameraEdgesDoNotOvershootOrReverse(t *testing.T) {
	for _, size := range []image.Point{{1664, 1280}, {810, 610}, {600, 400}} {
		w := &World{Scene: &Scene{Width: size.X, Height: size.Y}}
		previousX, previousY := 0, 0
		for p := 0; p <= max(size.X, size.Y)+100; p++ {
			w.Player.X, w.Player.Y = p, p
			x, y := w.Camera()
			if x < previousX || y < previousY || x > max(0, size.X-820) || y > max(0, size.Y-600) || x-previousX > 1 || y-previousY > 1 {
				t.Fatalf("map %v player %d: camera %d,%d after %d,%d", size, p, x, y, previousX, previousY)
			}
			previousX, previousY = x, y
		}
	}
}

func TestZoomedCameraAndScenePoint(t *testing.T) {
	w := &World{Scene: &Scene{Width: 3000, Height: 2000}, Player: Player{X: 1500, Y: 1000}, ViewWidth: 640, ViewHeight: 480}
	if x, y := w.Camera(); x != 1180 || y != 760 {
		t.Fatal("zoomed camera", x, y)
	}
	if x, y := w.ScenePoint(400, 300); x != 320 || y != 240 {
		t.Fatal("zoomed pointer", x, y)
	}
	w.Player.X, w.Player.Y = 3000, 2000
	if x, y := w.Camera(); x != 2340 || y != 1520 {
		t.Fatal("zoomed edge clamp", x, y)
	}
	w.ViewWidth, w.ViewHeight = 0, 0
	if x, y := w.ScenePoint(123, 234); x != 123 || y != 234 {
		t.Fatal("native pointer changed")
	}
}
