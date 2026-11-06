package world

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
)

func TestSceneryNativeSignedCoordinatesAndClipping(t *testing.T) {
	root := t.TempDir()
	// Independently authored WEM bytes: one 2x2 frame, sorted 20 pixels below
	// its signed Y. The coordinates below retain their unsigned wire bits.
	const wem = `{"entries":[{"name":"1.wem","decoded_hex":"01000000000000000100000001000002000200ff0300"}]}`
	if err := os.WriteFile(filepath.Join(root, "wem_data.json"), []byte(wem), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "pictures", "images1")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	picture := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			picture.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	f, err := os.Create(filepath.Join(dir, "1.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, picture); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	scene := &Scene{}
	placements := []groundObject{
		{Resource: 1, X: 0xffff, Y: 0xffff},
		{Resource: 1, X: 0x8000, Y: 0x7fff},
		{Resource: 1, X: 0x7fff, Y: 0x8000},
		{Resource: 1, X: 12, Y: 34},
	}
	if err := scene.loadObjects(login.NewAssets(root), placements); err != nil {
		t.Fatal(err)
	}
	if len(scene.Objects) != 4 {
		t.Fatalf("missing scenery fixtures: %d", len(scene.Objects))
	}
	for i, want := range []image.Point{{-1, -1}, {-32768, 32767}, {32767, -32768}, {12, 34}} {
		got := scene.Objects[i]
		if got.X != want.X || got.Y != want.Y || got.SortY() != want.Y+20 {
			t.Fatalf("placement %d: %d,%d depth %d, want %v depth %d", i, got.X, got.Y, got.SortY(), want, want.Y+20)
		}
	}
	canvas := surface.New(2, 2)
	scene.Objects[0].Draw(canvas, 0, 0, time.Time{})
	if canvas.Pix[0] != 0xffff || canvas.Pix[1] != 0 || canvas.Pix[2] != 0 || canvas.Pix[3] != 0 {
		t.Fatal("scenery crossing both map borders was not clipped into view", canvas.Pix)
	}
	if placements[0].X != 0xffff || placements[0].Y != 0xffff {
		t.Fatal("native placement bits were modified")
	}
}

// TestShipDeckObjects: the deck's record lists 72 objects; 11 lie under
// the characters (kind 1), 19 are depth-sorted with them (kind 3) and 42,
// the railings among them, are painted over them (kind 5).
func TestShipDeckObjects(t *testing.T) {
	needAssets(t)
	s, err := LoadScene(assets, 10017)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[byte]int{}
	for _, o := range s.Objects {
		kinds[o.Kind]++
		if o.Image == nil || o.Frames < 1 {
			t.Fatalf("object %d without a picture", o.Resource)
		}
	}
	if len(s.Objects) != 72 || kinds[1] != 11 || kinds[3] != 19 || kinds[5] != 42 {
		t.Fatalf("%d objects, kinds %v", len(s.Objects), kinds)
	}
}

// TestObjectFrames: a stacked picture shows one frame per interval, and
// the green key stays transparent.
func TestObjectFrames(t *testing.T) {
	img := surface.New(2, 4)
	img.Pix = []uint16{1, 0, 1, 1, 2, 2, 2, 2}
	o := Object{X: 10, Y: 10, Frames: 2, Interval: 500 * time.Millisecond, Image: img}
	dst := surface.New(20, 20)
	dst.Fill(image.Rect(0, 0, 20, 20), 9)
	o.Draw(dst, 0, 0, time.UnixMilli(0))
	if dst.Pix[10*20+10] != 1 || dst.Pix[10*20+11] != 9 {
		t.Fatalf("first frame %v", dst.Pix[10*20+10:10*20+12])
	}
	o.Draw(dst, 0, 0, time.UnixMilli(500))
	if dst.Pix[10*20+10] != 2 || dst.Pix[11*20+11] != 2 {
		t.Fatal("second frame not drawn")
	}
}
