package app

import (
	"bytes"
	"image"
	"os"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/surface"
)

func TestLeftCoconutClickOwnsMovement(t *testing.T) {
	c := testClient(t)
	now := time.Unix(100, 0)
	c.Now = func() time.Time { return now }
	c.dispatch(selfPacket(10001, 2, 11016, 562, 635, 0, 444444444, 444444444, nil, "Dango"))
	c.dispatch([]byte{5, 4})
	c.dispatch([]byte{23, 4, 3, 2, 0, 106, 160, 0, 0, 206, 1, 26, 2, 170, 0, 0, 0})
	hover := c.floorItemPoint(c.floorItems[2])
	c.Input.X, c.Input.Y = hover.X, hover.Y
	now = now.Add(time.Second)
	c.Frame()
	if c.floorHovered != 2 {
		t.Fatal("left Coconut hover not reached through the frame loop")
	}
	if path := os.Getenv("FLOOR_HOVER_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
	sent := wire(t, c)
	c.GroundClick(450, 400)
	if !c.World.Walking() || !c.groundHeld {
		t.Fatal("fixture did not start a held ground walk")
	}
	before := len(sent())
	c.pendingNPC = c.World.NPCs[1]
	at := c.floorItemPoint(c.floorItems[2])
	c.GroundClick(at.X, at.Y)
	if c.World.Walking() || c.groundHeld || c.pendingNPC != nil || c.floorPendingPickup != 0 || c.World.Marker.Shown() {
		t.Fatal("pickup did not take ownership from the previous walk")
	}
	if got := sent(); len(got) != before+2 || !bytes.Equal(got[before], []byte{6, 1, 4, 50, 2, 123, 2, 0, 0, 0, 0, 0, 0, 0, 0}) || !bytes.Equal(got[before+1], []byte{23, 2, 2, 0}) {
		t.Fatal("current position must precede the native Coconut pickup", got)
	}
	now = now.Add(time.Second)
	c.GroundHold(true, 700, 400)
	c.World.Step(now)
	if c.World.Walking() || c.World.Player.X != 562 || c.World.Player.Y != 635 {
		t.Fatal("held pointer reactivated movement after pickup")
	}
	if len(sent()) != before+2 {
		t.Fatal("held pointer sent another movement packet")
	}
}

func TestFloorPickupApproachRunsWithoutRendering(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch([]byte{23, 3, 106, 160, 120, 5, 51, 4, 0, 0, 0, 0, 1}) // Coconut at 1400,1075.
	sent := wire(t, c)
	at := c.floorItemPoint(c.floorItems[1])
	c.GroundClick(at.X, at.Y)
	if c.floorPendingPickup != 1 || c.groundHeld || !c.World.Walking() {
		t.Fatal("distant pickup did not own its approach")
	}
	for i := 0; i < 100 && c.floorPendingPickup != 0; i++ {
		*now = now.Add(100 * time.Millisecond)
		c.frame(false)
	}
	if c.floorPendingPickup != 0 || c.World.Walking() || c.World.Player.X == 1400 {
		t.Fatal("pickup did not stop within reach before the item position")
	}
	if got := sent(); len(got) != 2 || got[0][0] != 6 || !bytes.Equal(got[1], []byte{23, 2, 1, 0}) {
		t.Fatal("approach did not stop before sending exactly one pickup", got)
	}
}

func TestFloorPickupCancelledByNewGroundClick(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch([]byte{23, 3, 106, 160, 120, 5, 51, 4, 0, 0, 0, 0, 1})
	at := c.floorItemPoint(c.floorItems[1])
	c.GroundClick(at.X, at.Y)
	c.GroundClick(400, 400)
	if c.floorPendingPickup != 0 || !c.groundHeld {
		t.Fatal("new ground click left an old pickup pending")
	}
}

func TestFloorHoverHighlightsAndNamesVisiblePixels(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch([]byte{23, 3, 106, 160, 18, 4, 51, 4, 0, 0, 0, 0, 1})
	item := c.floorItems[1]
	at := c.floorItemPoint(item)
	unlit, lit := surface.New(c.Screen.W, c.Screen.H), surface.New(c.Screen.W, c.Screen.H)
	c.floorItemSprites()[0].Draw(unlit, at.X, at.Y)
	c.Input.X, c.Input.Y = at.X, at.Y
	c.updateFloorHover()
	if c.floorHovered != 1 {
		t.Fatal("opaque Coconut pixel not hovered")
	}
	c.floorItemSprites()[0].Draw(lit, at.X, at.Y)
	brightened := 0
	for i, p := range unlit.Pix {
		if p != 0 && lit.Pix[i] != p {
			brightened++
		}
		if p == 0 && lit.Pix[i] != 0 {
			t.Fatal("hover filled a transparent icon pixel")
		}
	}
	if brightened == 0 {
		t.Fatal("hover did not highlight artwork")
	}
	c.Screen.Fill(image.Rect(0, 0, c.Screen.W, c.Screen.H), 0)
	c.drawFloorItemName()
	yellow := 0
	for _, p := range c.Screen.Pix {
		if p == 0xffe0 {
			yellow++
		}
	}
	if yellow == 0 {
		t.Fatal("yellow item name missing")
	}
	bounds := c.groundItemBounds(item)
	if c.floorItemAt(bounds.Min.X, bounds.Min.Y) != 0 {
		t.Fatal("transparent corner intercepts walking")
	}
	c.Input.X, c.Input.Y = bounds.Min.X, bounds.Min.Y
	c.updateFloorHover()
	if c.floorHovered != 0 {
		t.Fatal("transparent corner retained hover")
	}
	c.Input.X, c.Input.Y = at.X, at.Y
	c.uiHovered = true
	c.updateFloorHover()
	if c.floorHovered != 0 {
		t.Fatal("UI did not suppress floor hover")
	}
}

func TestFloorHoverFollowsFrontmostItem(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch([]byte{23, 3, 106, 160, 18, 4, 51, 4, 0, 0, 0, 0, 1, 2})
	front := c.floorItems[2]
	front.depth = 10
	c.floorItems[2] = front
	at := c.floorItemPoint(front)
	if c.floorItemAt(at.X, at.Y) != 2 {
		t.Fatal("overlap selected a lower item")
	}
}
