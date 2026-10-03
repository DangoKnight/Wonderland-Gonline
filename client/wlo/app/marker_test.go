package app

import (
	"image"
	"testing"
	"time"
)

// TestWalkMarker: a mouse walk shows the marker at the route's end and
// steps Arrow2, Arrow3, Arrow4 every 120 ms, then hides; a click on
// unwalkable ground puts it on the walkable cell where the line to the
// player meets it; arrow-key walks show none.
func TestWalkMarker(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.GroundClick(300, 350)
	m := &c.World.Marker
	if !m.Shown() {
		t.Fatal("no marker after a click")
	}
	at := m.At()
	if !c.World.Walking() {
		t.Fatal("no walk")
	}
	pics := []int{c.Pics.Find("Arrow2"), c.Pics.Find("Arrow3"), c.Pics.Find("Arrow4")}
	for _, p := range pics {
		if p < 0 {
			t.Fatal("arrow pictures missing")
		}
	}
	c.GroundHold(false, 0, 0)
	c.Frame()
	if !m.Shown() {
		t.Fatal("marker gone on its first frame")
	}
	for i := 0; i < 3; i++ {
		*now = now.Add(121 * time.Millisecond)
		c.Frame()
	}
	if m.Shown() {
		t.Fatal("marker still up after Arrow4")
	}
	if at.X%20 != 2 || at.Y%20 != 15 {
		t.Fatalf("marker at %v, not on a waypoint", at)
	}

	// Far over the railing (down and right of the deck's edge).
	c.World.StopWalk()
	cx, cy := c.World.Camera()
	c.GroundClick(790, 590)
	c.GroundHold(false, 0, 0)
	end := m.At()
	if end == image.Pt(790+cx, 590+cy) {
		t.Fatal("marker left on the blocked point")
	}
	if !c.World.Scene.Walkable(end.X, end.Y) {
		t.Fatalf("marker at %v, not on walkable ground", end)
	}

	m.Hide()
	c.World.StopWalk()
	*now = now.Add(time.Second)
	c.WalkKeys(true, false, false, false)
	if m.Shown() {
		t.Fatal("arrow keys showed the marker")
	}
}
