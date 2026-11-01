package world

import "testing"

// TestTurnAction: FUN_00432638 keeps the action's family (FUN_00427150)
// and replaces its facing.
func TestTurnAction(t *testing.T) {
	for _, c := range []struct{ action, base int }{{3, 0}, {0xb, 8}, {0x11, 0x10}, {0x30, 0x2e}, {99, 99}, {0x4a, 0}} {
		if got := ActionBase(c.action); got != c.base {
			t.Errorf("ActionBase(%#x) = %#x, want %#x", c.action, got, c.base)
		}
	}
	if got := TurnAction(standingAction+FaceUp, 100, 100, 90, 100); got != standingAction+FaceLeft {
		t.Errorf("standing turned to %d", got)
	}
	if got := TurnAction(FaceUp, 100, 100, 100, 110); got != walkingAction+FaceDown {
		t.Errorf("walking turned to %d", got)
	}
}

// TestDirectionPoint: directions 1..8 run clockwise from up; others are
// unset.
func TestDirectionPoint(t *testing.T) {
	want := []int{FaceUp, FaceUpRight, FaceRight, FaceDownRight, FaceDown, FaceDownLeft, FaceLeft, FaceUpLeft}
	for d := 1; d <= 8; d++ {
		x, y, ok := DirectionPoint(100, 100, d)
		if !ok || Facing(100, 100, x, y, -1) != want[d-1] {
			t.Errorf("direction %d: (%d, %d) %v", d, x, y, ok)
		}
	}
	if _, _, ok := DirectionPoint(100, 100, 0); ok {
		t.Error("direction 0 named a point")
	}
}
