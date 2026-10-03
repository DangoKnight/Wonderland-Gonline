package cursor

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func exported(t *testing.T) string {
	dir := filepath.Join("..", "..", "..", "data", "media", "cursor")
	if _, err := os.Stat(filepath.Join(dir, "cursor1", "manifest.json")); err != nil {
		t.Skip("cursor exports not present")
	}
	return dir
}

func TestLoadAllSelectsNormal(t *testing.T) {
	now := time.Unix(0, 0)
	c, err := LoadAll(exported(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Shapes) != len(Registered) || c.Current != ShapeNormal {
		t.Fatalf("%d shapes, current %d", len(c.Shapes), c.Current)
	}
	if c.Shapes[ShapePoint] != c.Shapes[ShapePointAlt] {
		t.Fatal("cursor4.ani is registered twice but should load once")
	}
	f := c.Frame(now)
	if f == nil || f.Image.Bounds().Dx() != 32 || f.Hotspot != (image.Point{}) {
		t.Fatalf("normal cursor frame %+v", f)
	}
	if g := c.Shapes[ShapeGrab].Frames; g[0].Hotspot != image.Pt(8, 25) || g[1].Hotspot != image.Pt(7, 25) {
		t.Fatal("cursor8 keeps a hotspot per frame")
	}
}

// TestTiming checks ANI playback: cursor1 shows eight frames at 10
// jiffies (1/6 s) each, then loops.
func TestTiming(t *testing.T) {
	a, err := Load(exported(t), "cursor1")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		d    time.Duration
		want int
	}{{0, 0}, {10*jiffy - 1, 0}, {10 * jiffy, 1}, {75 * jiffy, 7}, {80 * jiffy, 0}, {95 * jiffy, 1}} {
		if got := a.At(tc.d); got != tc.want {
			t.Errorf("At(%v) = %d, want %d", tc.d, got, tc.want)
		}
	}
}

func TestSetKeepsRunningAnimation(t *testing.T) {
	start := time.Unix(0, 0)
	a := &Animation{Frames: make([]Frame, 2), Sequence: []int{0, 1}, Rates: []int{1, 1}, period: 2 * jiffy}
	c := &Cursors{Shapes: map[Shape]*Animation{ShapeNormal: a, ShapeBusy: a}}
	c.Set(ShapeNormal, start)
	c.Set(ShapeNormal, start.Add(jiffy))
	if c.Frame(start.Add(jiffy)) != &a.Frames[1] {
		t.Fatal("re-selecting the same shape restarted it")
	}
	c.Set(ShapeBusy, start.Add(jiffy))
	if c.Frame(start.Add(jiffy)) != &a.Frames[0] {
		t.Fatal("a new shape should start at its first frame")
	}
	c.Set(Shape(50), start)
	if c.Frame(start) != nil {
		t.Fatal("an unregistered shape has no frame")
	}
}
