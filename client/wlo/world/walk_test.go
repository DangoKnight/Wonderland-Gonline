package world

import (
	"image"
	"testing"
	"time"
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
