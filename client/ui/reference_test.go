package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testAssets(t *testing.T) *Assets {
	t.Helper()
	a, err := NewAssets(filepath.Join("..", "..", "..", "Wonderland-Client"))
	if err != nil {
		t.Skip("client assets not installed:", err)
	}
	return a
}

// compare reports pixels of got that differ from a reference capture by more
// than tolerance per channel, writing a diff image when DIFF_DIR is set.
func compare(t *testing.T, name string, got *image.RGBA, tolerance int) (bad int) {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "reference", "screenshots", "Login", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ref, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	diff := image.NewRGBA(got.Rect)
	var hist [256]int
	for y := range ScreenHeight {
		for x := range ScreenWidth {
			// The captures include the window frame's rounded bottom corners.
			if y >= ScreenHeight-5 && (x < 5 || x >= ScreenWidth-5) {
				continue
			}
			r, g, b, _ := ref.At(x, y).RGBA()
			c := got.RGBAAt(x, y)
			d := max(absInt(int(r>>8)-int(c.R)), absInt(int(g>>8)-int(c.G)), absInt(int(b>>8)-int(c.B)))
			hist[d]++
			if d > tolerance {
				bad++
				diff.Pix[diff.PixOffset(x, y)+0] = 255
				diff.Pix[diff.PixOffset(x, y)+3] = 255
			} else {
				o := diff.PixOffset(x, y)
				diff.Pix[o], diff.Pix[o+1], diff.Pix[o+2], diff.Pix[o+3] = c.R/3, c.G/3, c.B/3, 255
			}
		}
	}
	if dir := os.Getenv("DIFF_DIR"); dir != "" {
		for n, m := range map[string]*image.RGBA{"got": got, "diff": diff} {
			out, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s.%s.png", name, n)))
			if err == nil {
				png.Encode(out, m)
				out.Close()
			}
		}
		t.Logf("%s: max channel difference histogram (first 16): %v", name, hist[:16])
	}
	return bad
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestServerSelectMatchesOriginal(t *testing.T) {
	a := testAssets(t)
	regions, _ := ParseServerINI([]byte("01[Local]1\r\nLocal1*127.0.0.1\r\n"))
	s := NewServerSelect(regions)
	s.Signals[101] = SignalNormal
	frame := NewFrame()
	for _, tc := range []struct {
		ref    string
		region int
	}{{"server_select.png", -1}, {"server_select_region.png", 0}} {
		if tc.region >= 0 {
			s.SelectRegion(tc.region)
		}
		if err := s.Draw(a, frame); err != nil {
			t.Fatal(err)
		}
		if bad := compare(t, tc.ref, frame, 0); bad > 0 {
			t.Errorf("%s: %d pixels differ", tc.ref, bad)
		}
	}
}

func TestLoginMatchesOriginal(t *testing.T) {
	a := testAssets(t)
	l := NewLogin()
	l.shown = time.Now()
	frame := NewFrame()
	if err := l.Draw(a, frame); err != nil {
		t.Fatal(err)
	}
	if bad := compare(t, "login.png", frame, 0); bad > 0 {
		t.Errorf("%d pixels differ", bad)
	}
}
