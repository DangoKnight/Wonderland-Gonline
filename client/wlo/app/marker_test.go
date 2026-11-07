package app

import (
	"image"
	"testing"
	"time"
)

func TestPlacementDoesNotShowWalkMarker(t *testing.T) {
	fixture := newClientFixture(t)
	for _, tc := range []struct {
		name   string
		packet []byte
	}{
		{"login position", []byte{7, 0x11, 0x27, 0, 0, 0x21, 0x27, 0x12, 4, 0x33, 4}},
		{"same map warp", []byte{12, 0x11, 0x27, 0, 0, 0x21, 0x27, 0x12, 4, 0x33, 4, 0, 0, 0}},
		{"map switch", []byte{12, 0x11, 0x27, 0, 0, 0x2b, 0x27, 0xa2, 2, 0x2b, 4, 0, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := enteredClient(t, fixture)
			if c.World.Marker.Shown() {
				t.Fatal("login showed a waypoint")
			}
			c.dispatch(tc.packet)
			if c.World.Marker.Shown() {
				t.Fatal("server placement showed a waypoint")
			}
			// A later placement must also clear an existing held walk cue.
			c.World.Marker.Show(image.Pt(c.World.Player.X, c.World.Player.Y))
			c.groundHeld = true
			c.dispatch(tc.packet)
			if c.World.Marker.Shown() || c.groundHeld || c.World.Walking() {
				t.Fatal("server placement retained walking presentation")
			}
		})
	}
}

// TestWalkMarker: a mouse walk shows the marker at the route's end and
// animates for 600 ms after release, then hides; a click on
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
	c.GroundHold(false, 0, 0)
	c.Frame()
	if !m.Shown() {
		t.Fatal("marker gone on its first frame")
	}
	for i := 0; i < 5; i++ {
		*now = now.Add(121 * time.Millisecond)
		c.Frame()
	}
	if m.Shown() {
		t.Fatal("marker still up after its animation duration")
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

// Waypoint_Legacy keeps the marker at the player's feet after reaching the
// walkable edge, including subsequent clicks and held-button re-aims.
func TestWalkMarkerBlockedArrival(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	cx, cy := c.World.Camera()
	target := image.Pt(790+cx, 590+cy)
	c.GroundClick(790, 590)
	end := c.World.Marker.At()
	if !c.World.Walking() || end == target {
		t.Fatal("blocked click failed to plan a walk to the edge")
	}
	c.GroundHold(false, 0, 0)
	*now = now.Add(10 * time.Second)
	c.World.Step(*now)
	if c.World.Walking() || image.Pt(c.World.Player.X, c.World.Player.Y) != end {
		t.Fatal("player failed to arrive at the walkable edge")
	}
	sent() // discard the original route's movement packets
	cx, cy = c.World.Camera()
	c.GroundClick(target.X-cx, target.Y-cy)
	if c.World.Walking() || c.World.Marker.At() != end {
		t.Fatalf("repeated blocked click moved marker: got %v want %v", c.World.Marker.At(), end)
	}
	*now = now.Add(401 * time.Millisecond)
	c.GroundHold(true, target.X-cx, target.Y-cy)
	if c.World.Walking() || !c.World.Marker.Shown() || c.World.Marker.At() != end {
		t.Fatal("held blocked click did not retain marker at reachable feet")
	}
	if packets := sent(); len(packets) != 0 {
		t.Fatalf("stationary blocked clicks sent movement: %x", packets)
	}
	c.GroundHold(false, 0, 0)
}

func TestWalkMarkerFollowsHeldCursor(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.GroundClick(300, 350)
	deadline := c.nextWalk
	*now = now.Add(16 * time.Millisecond)
	c.GroundHold(true, 320, 360)
	cx, cy := c.World.Camera()
	if c.World.Marker.At() != image.Pt(320+cx, 360+cy) {
		t.Fatal("marker waited for walk repeat")
	}
	if c.nextWalk != deadline {
		t.Fatal("visual tracking replanned movement early")
	}
	c.GroundHold(true, 790, 590)
	if at := c.World.Marker.At(); !c.World.Scene.Walkable(at.X, at.Y) {
		t.Fatal("held marker followed cursor outside walkable area")
	}
	if c.nextWalk != deadline {
		t.Fatal("blocked marker tracking replanned movement early")
	}
	c.GroundHold(false, 0, 0)
	at := c.World.Marker.At()
	c.GroundHold(false, 500, 500)
	if c.World.Marker.At() != at {
		t.Fatal("released marker followed cursor")
	}
}

func TestHeldMarkerUsesDrawingCamera(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.GroundClick(300, 350)
	c.GroundHold(true, 320, 360)
	cx, cy := c.World.Camera()
	// The camera changes after input is sampled, as it does when Frame steps
	// the player. Rendering must retain the exact screen-space pointer anchor.
	camera := image.Pt(cx+2, cy+1)
	c.World.CameraAt = &camera
	*now = now.Add(16 * time.Millisecond)
	c.Frame()
	if got := c.World.Marker.At().Sub(camera); got != image.Pt(320, 360) {
		t.Fatalf("held marker shifted with drawing camera: %v", got)
	}
	c.GroundHold(false, 320, 360)
	at := c.World.Marker.At()
	camera = camera.Add(image.Pt(2, 1))
	*now = now.Add(16 * time.Millisecond)
	c.Frame()
	if c.World.Marker.At() != at {
		t.Fatal("released marker followed camera instead of remaining on ground")
	}
}
