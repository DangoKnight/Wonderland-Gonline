package assets

import "testing"

func TestVehicleLandingRequiresClearLand(t *testing.T) {
	terrain := Terrain{Width: 100, Height: 20, GridWidth: 5, GridHeight: 1, Cells: []byte{0, 2, 8, 1, 16}}
	for _, tc := range []struct {
		x, y int
		land bool
	}{{1, 1, true}, {21, 1, false}, {41, 1, false}, {61, 1, false}, {81, 1, false}, {-1, 1, false}, {100, 1, false}, {1, 20, false}} {
		if got := terrain.Land(tc.x, tc.y); got != tc.land {
			t.Fatalf("land at %d,%d = %v", tc.x, tc.y, got)
		}
	}
}

func TestNearestWalkableTerrain(t *testing.T) {
	t0 := Terrain{Width: 100, Height: 100, GridWidth: 5, GridHeight: 5, Cells: make([]byte, 25)}
	for i := range t0.Cells {
		t0.Cells[i] = 2
	}
	t0.Cells[1*5+2] = 0
	t0.Cells[4*5+2] = 0
	t0.Cells[2*5+2] = 8 // Alternate water must not be mistaken for safe land.
	x, y, ok := t0.NearestWalkable(50, 50)
	if !ok || x != 39 || y != 50 || !t0.Walkable(int(x), int(y)) {
		t.Fatalf("nearest land %d,%d %v", x, y, ok)
	}
	if x, y, ok := t0.NearestWalkable(25, 45); !ok || x != 25 || y != 45 {
		t.Fatal("walkable position changed")
	}
	for i := range t0.Cells {
		t0.Cells[i] = 2
	}
	if _, _, ok := t0.NearestWalkable(50, 50); ok {
		t.Fatal("all-water terrain returned land")
	}
}
