package game

import "testing"

func TestRaftPlacementRequiresAdjacentNavigableWater(t *testing.T) {
	for _, tc := range []struct {
		name  string
		water [][2]int
		want  bool
	}{
		{"dry land", nil, false},
		{"isolated puddle", [][2]int{{130, 110}}, false},
		{"distant water", [][2]int{{150, 110}, {170, 110}}, false},
		{"blocked corner", [][2]int{{130, 130}, {150, 130}}, false},
		{"open shore", [][2]int{{130, 110}, {150, 110}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			water := map[[2]int]bool{}
			for _, p := range tc.water {
				water[p] = true
			}
			land := func(x, y int) bool { return x == 110 && y == 110 }
			x, y, ok := RaftPlacement(110, 110, land, func(x, y int) bool { return water[[2]int{x, y}] })
			if ok != tc.want || ok && (x != 130 || y != 110) {
				t.Fatal(x, y, ok)
			}
		})
	}
}
