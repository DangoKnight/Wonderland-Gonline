package app

import (
	"testing"
	"time"

	"wonderland-gonline/client/wlo/cursor"
)

// TestLoginCursor: over the password field the cursor is the pointing
// hand (TSe_Component's state 2, cursor4.ani); over the account form's
// empty area it is the normal cursor1.ani, as in the Login captures
// Hand_Cursor.png and Regular_Cursor.png.
func TestLoginCursor(t *testing.T) {
	c := testClient(t)
	now := time.Now()
	c.Now = func() time.Time { return now }
	c.Frame()
	c.Servers.Hide()
	c.Login.Show()
	c.Frame() // the form takes its place
	p := c.Login.Password.Base().Rect()
	c.Input.X, c.Input.Y = p.Min.X+p.Dx()/2, p.Min.Y+p.Dy()/2
	c.Frame()
	if c.Cursors.Current != cursor.ShapePoint {
		t.Fatalf("over the password field: shape %d", c.Cursors.Current)
	}
	f := c.Login.Base().Rect()
	c.Input.X, c.Input.Y = f.Max.X-12, f.Min.Y+f.Dy()/2
	c.Frame()
	if c.Cursors.Current != cursor.ShapeNormal {
		t.Fatalf("over the form: shape %d", c.Cursors.Current)
	}
}

// TestNPCHover: an NPC under the pointer is drawn lit and the cursor stays
// normal; holding the button to walk turns it into cursor2.ani after a
// second.
func TestNPCHover(t *testing.T) {
	c, now, _ := enteredClient(t)
	n, r := nearNPC(t, c)
	c.Input.X, c.Input.Y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	c.Frame()
	if c.World.Hovered() != n {
		t.Fatalf("hovered %v, want click %d", c.World.Hovered(), n.ClickID)
	}
	if c.Cursors.Current != cursor.ShapeNormal {
		t.Fatalf("over an NPC: shape %d", c.Cursors.Current)
	}
	c.Input.X, c.Input.Y = 300, 400
	c.Frame()
	if c.World.Hovered() != nil {
		t.Fatal("still hovered off the NPC")
	}
	c.GroundClick(300, 400)
	*now = now.Add(heldWalkArrow)
	c.Frame()
	if c.Cursors.Current != cursor.ShapeNormalAlt {
		t.Fatalf("held walk: shape %d", c.Cursors.Current)
	}
	c.GroundHold(false, 0, 0)
	c.Frame()
	if c.Cursors.Current != cursor.ShapeNormal {
		t.Fatalf("after release: shape %d", c.Cursors.Current)
	}
}
