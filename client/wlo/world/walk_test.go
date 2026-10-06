package world

import (
	"image"
	"testing"
	"time"
	"wonderland-gonline/internal/clientassets"
)

// TestFacing checks FUN_0041218c's eight directions and its 0.25..2 band
// for diagonals.
func TestFacing(t *testing.T) {
	for _, tc := range []struct {
		dx, dy, want int
	}{
		{0, -10, FaceUp}, {-10, -10, FaceUpLeft}, {-10, 0, FaceLeft}, {-10, 10, FaceDownLeft},
		{0, 10, FaceDown}, {10, 10, FaceDownRight}, {10, 0, FaceRight}, {10, -10, FaceUpRight},
		{100, 20, FaceRight}, {100, 30, FaceDownRight}, {10, 30, FaceDown}, {0, 0, FaceLeft},
	} {
		if got := Facing(0, 0, tc.dx, tc.dy, FaceLeft); got != tc.want {
			t.Errorf("(%d,%d): %d, want %d", tc.dx, tc.dy, got, tc.want)
		}
	}
}

// TestShipDeckWalk: on Ship Deck a click across the deck gives a path of
// walkable waypoints; walking it sends one 6/1 per leg and ends standing
// at the last waypoint.
func TestShipDeckWalk(t *testing.T) {
	needAssets(t)
	s, err := LoadScene(assets, 10017)
	if err != nil {
		t.Fatal(err)
	}
	path := s.Plan(1042, 1075, 700, 1150)
	if len(path) == 0 {
		t.Fatal("no path across the deck")
	}
	for _, p := range path {
		if c := cellOf(p.X, p.Y); !s.free(c.X, c.Y) {
			t.Fatalf("waypoint %v on a blocked cell", p)
		}
	}
	if s.Plan(1042, 1075, 1042+walkReach*cellSize, 1075) != nil {
		t.Fatal("a target 41 cells away was accepted")
	}

	now := time.Unix(0, 0)
	w := &World{Scene: s, Player: Player{X: 1042, Y: 1075, Direction: standingFront}}
	var legs []image.Point
	w.OnLeg = func(_, x, y int) { legs = append(legs, image.Pt(x, y)) }
	if !w.WalkTo(700, 1150, now) {
		t.Fatal("walk refused")
	}
	if w.Player.Direction >= standingAction {
		t.Fatalf("walking with action %d", w.Player.Direction)
	}
	for i := 0; i < 600 && w.Walking(); i++ {
		now = now.Add(16 * time.Millisecond)
		w.Step(now)
	}
	last := w.Scene.Plan(1042, 1075, 700, 1150)
	end := last[len(last)-1]
	if w.Walking() || w.Player.X != end.X || w.Player.Y != end.Y || len(legs) != len(last) {
		t.Fatalf("ended at (%d,%d) after %d legs, want %v after %d", w.Player.X, w.Player.Y, len(legs), end, len(last))
	}
	if w.Player.Direction < standingAction {
		t.Fatal("not standing at the end")
	}
}

// Verify a real Newbie Island shore using the installed WLRI terrain export.
func TestNewbieIslandWaterRoute(t *testing.T) {
	needAssets(t)
	s, err := LoadScene(assets, 10003)
	if err != nil {
		t.Fatal(err)
	}
	for x := 1; x < int(s.Ground.GridWidth)-1; x++ {
		for y := 1; y < int(s.Ground.GridHeight)-1; y++ {
			from := waypoint(image.Pt(x, y))
			if !s.Walkable(from.X, from.Y) {
				continue
			}
			for _, d := range []image.Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				to := waypoint(image.Pt(x, y).Add(d))
				if !s.Water(to.X, to.Y) {
					continue
				}
				land := s.Plan(from.X, from.Y, to.X, to.Y)
				if len(land) > 0 && s.Water(land[len(land)-1].X, land[len(land)-1].Y) {
					t.Fatal("walked into water without vehicle")
				}
				boarding, ok := s.ShorePoint(from.X, from.Y, true)
				if !ok {
					continue
				}
				water := s.PlanWater(boarding.X, boarding.Y, to.X, to.Y)
				if !s.SameCell(boarding, to) && (len(water) == 0 || !s.Water(water[len(water)-1].X, water[len(water)-1].Y)) {
					continue
				}
				if !s.WaterEdge(from.X, from.Y) || s.WaterTravel {
					t.Fatal("shore detection changed terrain state")
				}
				t.Logf("verified Newbie Island shoreline %v -> %v", from, to)
				return
			}
		}
	}
	t.Fatal("no usable water shoreline found in Newbie Island terrain")
}

func TestWaterPathStopsAtLand(t *testing.T) {
	cells := make([]byte, 100)
	for x := 2; x < 10; x++ {
		for y := 0; y < 10; y++ {
			cells[x*10+y] = 2
		}
	}
	s := &Scene{WaterTravel: true, Ground: clientassets.GroundPrefix{GridWidth: 10, GridHeight: 10, Cells: cells}}
	if s.Walkable(22, 55) || !s.Walkable(82, 55) {
		t.Fatal("wrong water collision mode")
	}
	path := s.Plan(82, 55, 22, 55)
	if len(path) == 0 || !s.Water(path[len(path)-1].X, path[len(path)-1].Y) {
		t.Fatal("water path crossed onto land", path)
	}
}
