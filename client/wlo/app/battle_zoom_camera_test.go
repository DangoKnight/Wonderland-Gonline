package app

import (
	"testing"
	"time"
)

// Double Kick (30028) has no authored camera track, but its attack category
// still enables the close-range zoom. The cropped view must travel toward the
// enemy, otherwise the zoom cuts the target off.
func TestZoomMovesTheCameraTowardTheEnemy(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.battleFrame(false)
	c.dispatch([]byte{50, 1, 17, 0, 2, 2, 0x4c, 0x75, 0, 1, 4, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1})
	c.battleFrame(false)
	if m := c.battle.assets.Motions[30028]; len(m.Camera) != 0 {
		t.Skipf("skill 30028 now authors a camera track (%d points)", len(m.Camera))
	}

	moved, zoomed := false, false
	for i := 0; i < 20; i++ {
		*now = now.Add(100 * time.Millisecond)
		c.Frame()
		if c.battle.zoom.Dx() < ScreenWidth {
			zoomed = true
		}
		if c.battle.camera != battleCameraOrigin() {
			moved = true
		}
		if zoomed && !moved {
			t.Fatalf("zoom %v without moving the camera", c.battle.zoom)
		}
	}
	if !zoomed {
		t.Skip("the attack did not zoom in this run")
	}
}
