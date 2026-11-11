package app

import (
	"testing"
	"time"
)

// Two attacks can start in the same batch. The camera must follow the one that
// authors a camera track: before this, the first playing action owned the camera
// even when its motion had none, so the enemy's panning attack played as a plain
// zoom (session 2026-11-10 21:28).
func TestBattleCameraFollowsTheActionWithACamera(t *testing.T) {
	c, now, _ := enterBattleWith(t, newClientFixture(t))
	c.battleFrame(false)
	// Double Kick (30028) has no camera track; Hand Attack (10001) does.
	for _, p := range [][]byte{
		{50, 1, 17, 0, 4, 2, 0x4c, 0x75, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1},
		{50, 1, 17, 0, 2, 2, 0x11, 0x27, 0, 1, 4, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1},
	} {
		c.dispatch(p)
	}
	c.battleFrame(false)

	if _, ok := c.battle.assets.Motions[30028]; !ok {
		t.Fatal("required skill 30028 fixture is not loaded")
	}
	if m := c.battle.assets.Motions[30028]; len(m.Camera) != 0 {
		t.Fatalf("required no-track fixture changed (%d camera points)", len(m.Camera))
	}

	moved := false
	for i := 0; i < 10; i++ {
		*now = now.Add(100 * time.Millisecond)
		c.Frame()
		if c.battle.camera != battleCameraOrigin() {
			moved = true
			break
		}
	}
	if !moved {
		t.Fatal("the camera stayed at the origin while an attack with a camera track played")
	}
}
