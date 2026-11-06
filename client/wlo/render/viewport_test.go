package render

import (
	"math"
	"testing"
)

func TestViewportPreservesAspectAndPointerMapping(t *testing.T) {
	for _, tc := range []struct {
		name        string
		w, h        int
		scale, x, y float64
	}{
		{"native", 1020, 600, 1, 0, 0},
		{"1080p", 1920, 1080, 1.8, 42, 0},
		{"ultrawide", 3440, 1440, 2.4, 496, 0},
		{"portrait", 1020, 1200, 1, 0, 300},
		{"double", 2040, 1200, 2, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := FitViewport(1020, 600, tc.w, tc.h)
			if math.Abs(v.Scale-tc.scale) > 1e-9 || math.Abs(v.X-tc.x) > 1e-9 || math.Abs(v.Y-tc.y) > 1e-9 {
				t.Fatal(v)
			}
			x, y, inside := v.Point(tc.w/2, tc.h/2)
			if !inside || x != 510 || y != 300 {
				t.Fatal("center", x, y, inside)
			}
			for _, p := range [][2]int{{-1, 0}, {tc.w, tc.h / 2}, {tc.w / 2, tc.h}} {
				if _, _, ok := v.Point(p[0], p[1]); ok {
					t.Fatal("outside pointer accepted", p)
				}
			}
			if tc.x > 0 {
				if _, _, ok := v.Point(0, tc.h/2); ok {
					t.Fatal("side bar is interactive")
				}
			}
			if tc.y > 0 {
				if _, _, ok := v.Point(tc.w/2, 0); ok {
					t.Fatal("top bar is interactive")
				}
			}
		})
	}
}
