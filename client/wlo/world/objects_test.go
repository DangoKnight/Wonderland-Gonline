package world

import (
	"image"
	"testing"
	"time"

	"wonderland-go/client/wlo/surface"
)

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
