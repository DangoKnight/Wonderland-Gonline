package app

import (
	"image"
	"testing"
	"time"
)

func TestMovementModeHoldAndRelease(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.GroundClick(300, 350)
	*now = now.Add(movementModeHoldDuration - time.Millisecond)
	c.GroundHold(true, 320, 360)
	if c.movementMode {
		t.Fatal("latched before three seconds")
	}
	*now = now.Add(time.Millisecond)
	c.GroundHold(true, 320, 360)
	if !c.movementMode {
		t.Fatal("did not latch at three seconds")
	}
	*now = now.Add(walkHoldRepeat)
	c.GroundHold(false, 400, 400)
	cx, cy := c.World.Camera()
	if !c.groundHeld || !c.movementMode || c.World.Marker.At() != image.Pt(400+cx, 400+cy) {
		t.Fatal("release stopped cursor following")
	}
	c.GroundHold(false, 790, 590)
	at := c.World.Marker.At()
	if !c.World.Scene.Walkable(at.X, at.Y) {
		t.Fatal("latched marker left walkable ground")
	}
}

func TestMovementModeExitConsumesClick(t *testing.T) {
	fixture := newClientFixture(t)
	for _, button := range []string{"left", "right", "toolbar", "outside", "ground left", "ground right"} {
		t.Run(button, func(t *testing.T) {
			c, now, sent := enteredClient(t, fixture)
			c.GroundClick(300, 350)
			*now = now.Add(movementModeHoldDuration)
			c.GroundHold(true, 320, 360)
			g := NewGame(c)
			before := len(*sent)
			switch button {
			case "left":
				g.leftPress(0, 400, 400)
			case "right":
				g.rightPress(0, 400, 400)
			case "toolbar":
				r := c.FuncButtons.Buttons[pkButton].Base().Rect()
				g.leftPress(0, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
			case "outside":
				g.leftPress(0, -100, -100)
			case "ground left":
				c.GroundClick(400, 400)
			case "ground right":
				c.GroundRightClick(400, 400)
			}
			if c.movementMode || c.groundHeld || c.World.Walking() || c.pkTarget || len(*sent) != before {
				t.Fatal("exit click walked or retained movement mode")
			}
			*now = now.Add(movementModeHoldDuration)
			c.GroundHold(true, 500, 400)
			if c.groundHeld || c.movementMode || len(*sent) != before {
				t.Fatal("exit press rearmed movement before a new ground click")
			}
		})
	}
}

func TestMovementModeNPCHoverDoesNotInteract(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.GroundClick(300, 350)
	*now = now.Add(movementModeHoldDuration)
	c.GroundHold(true, 320, 360)
	n, r := nearNPC(t, c)
	x, y := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	c.GroundHold(false, x, y)
	c.World.Hover(x, y)
	if c.World.Hovered() != n || c.pendingNPC != nil || c.event.active || !c.movementMode {
		t.Fatal("latched movement failed to highlight the NPC without interacting")
	}
}

func TestMovementModeHoverDoesNotPickUp(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch([]byte{23, 3, 106, 160, 18, 4, 51, 4, 0, 0, 0, 0, 1})
	c.GroundClick(300, 350)
	*now = now.Add(movementModeHoldDuration)
	c.GroundHold(true, 320, 360)
	at := c.floorItemPoint(c.floorItems[1])
	c.Input.X, c.Input.Y = at.X, at.Y
	c.GroundHold(false, at.X, at.Y)
	c.updateFloorHover()
	if c.floorHovered != 1 || c.floorPendingPickup != 0 || c.pendingNPC != nil || !c.movementMode {
		t.Fatal("hover lost highlighting or triggered an interaction")
	}
}

func TestMovementModeInterrupted(t *testing.T) {
	fixture := newClientFixture(t)
	for _, reason := range []string{"release early", "event", "session", "placement"} {
		t.Run(reason, func(t *testing.T) {
			c, now, _ := enteredClient(t, fixture)
			c.GroundClick(300, 350)
			if reason != "release early" {
				*now = now.Add(movementModeHoldDuration)
				c.GroundHold(true, 320, 360)
			}
			switch reason {
			case "release early":
				c.GroundHold(false, 320, 360)
			case "event":
				c.event.active = true
				c.GroundHold(false, 320, 360)
			case "session":
				c.cancelSessionInput()
			case "placement":
				c.dispatch([]byte{7, 0x11, 0x27, 0, 0, 0x21, 0x27, 0x12, 4, 0x33, 4})
			}
			if c.groundHeld || c.movementMode || !c.groundSince.IsZero() {
				t.Fatal("movement mode survived interruption")
			}
		})
	}
}
