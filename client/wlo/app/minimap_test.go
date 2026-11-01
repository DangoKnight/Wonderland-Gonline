package app

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// TestMiniMap: Newbie Island (SceneData +0x1d = 1) shows its 142 × 154
// picture at the top right, 0x26 down, scaled by the map width; Ship Deck
// has none; Minimize swaps in the small form around the button and
// Expand restores the picture where it was.
func TestMiniMap(t *testing.T) {
	c, now, _ := enteredClient(t)
	m := c.MiniMap
	if m.Visible || m.Shown() {
		t.Fatal("Ship Deck shows a minimap")
	}
	c.dispatch(selfPacket(10001, 4, 10003, 1182, 2255, 7, 444444444, 444444444, nil, "Dango"))
	*now = now.Add(20 * time.Millisecond)
	c.Frame()
	if !m.Visible || m.Left != ScreenWidth-142 || m.Top != 0x26 || m.Width != 142 || m.Height != 154 {
		t.Fatalf("minimap at (%d, %d) %d × %d, visible %v", m.Left, m.Top, m.Width, m.Height, m.Visible)
	}
	if want := c.World.Scene.Width / 142; m.Scale != want || want == 0 {
		t.Fatalf("scale %d, want %d", m.Scale, want)
	}
	// The server marks an NPC with an unfinished quest branch: AC22:12
	// kind 1, click ID 8, sign 7.
	c.dispatch([]byte{protocol.CommandScene, protocol.SceneActorAction, 1, 8, 0, 7})
	signs := c.World.MiniMapSigns()
	if n := c.World.NPCs[8]; len(signs) != 1 || signs[0].Sign != 7 || signs[0].At != image.Pt(n.X, n.Y) {
		t.Fatalf("signs %+v", signs)
	}
	if c.Pics.Find(world.SignPicture+"7") < 0 {
		t.Fatal("no Icon_Sign_7")
	}
	right := m.Left + m.Minimize.Right()
	m.Toggle()
	if !m.Minimized || m.Width != 0x27 || m.Height != 0x26 || m.Reload.Visible {
		t.Fatalf("minimized %v %d × %d", m.Minimized, m.Width, m.Height)
	}
	m.Toggle()
	if m.Minimized || m.Left+m.Width != right || m.Top != 0x26 || !m.Reload.Visible {
		t.Fatalf("expanded at (%d, %d) width %d, want right edge %d", m.Left, m.Top, m.Width, right)
	}
}

// TestMiniMapPoint: FUN_0030f008 divides by the scale, moves three pixels
// left and raises by the band's k - round(y/height × k).
func TestMiniMapPoint(t *testing.T) {
	x, y := world.MiniMapPoint(1182, 2255, 20, 658, 0x26, 3000)
	// 2255 is in the third band (k 10): 10 - round(7.52) = 2.
	if x != 1182/20+658-3 || y != 2255/20+0x26-2 {
		t.Fatalf("(%d, %d)", x, y)
	}
}

// TestMiniMapSnapshot renders Newbie Island's minimap
// (In-Game/Newbie_Island_Props.png, top right).
func TestMiniMapSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, now, _ := enteredClient(t)
	c.dispatch(selfPacket(10001, 4, 10003, 1182, 2255, 7, 444444444, 444444444, nil, "Dango"))
	for _, id := range []byte{1, 2, 8} {
		c.dispatch([]byte{protocol.CommandScene, protocol.SceneActorAction, 1, id, 0, 7})
	}
	for i := 0; i < 30; i++ {
		*now = now.Add(20 * time.Millisecond)
		c.Frame()
	}
	save(t, c, filepath.Join(dir, "minimap.png"))
}
