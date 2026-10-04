package app

import (
	"time"

	"wonderland-go/client/wlo/cursor"
)

// The cursor (Tjo_Cursor, FUN_003bab68 each frame): a control under the
// pointer asks for its own state (+0xc5 via FUN_00467148 → FUN_003ba9e8),
// otherwise the game's state applies (FUN_003ba9d8, 0 outside special
// modes). FUN_003bac58 turns a state into a Screen.Cursors index: state 0
// is the normal cursor, cursor2.ani instead of cursor1.ani once a held
// button has walked the player for a second (DAT_00828ae0, set in the main
// loop's walk branch); every other state is its own index (2 is the
// pointing hand of TSe_Component; forms and fixed forms keep 0). The talk
// window is a component too. The game's other states (7, 6, 8 … for
// skills, trading and the like) are not ported.
const heldWalkArrow = time.Second

// updateCursor picks the cursor for this frame.
func (c *Client) updateCursor(now time.Time) {
	if c.Cursors == nil {
		return
	}
	state := byte(0)
	if h, ok := c.Input.Hovered.(interface{ CursorState() byte }); ok {
		state = h.CursorState()
	} else if c.Talk != nil && c.Talk.Contains(c.Input.X, c.Input.Y) {
		state = componentCursor
	}
	shape := cursor.Shape(state)
	if state == 0 && c.sportCursor() != 0 {
		shape = c.sportCursor()
	} else if state == 0 {
		shape = cursor.ShapeNormal
		if c.groundHeld && !c.groundSince.IsZero() && now.Sub(c.groundSince) >= heldWalkArrow {
			shape = cursor.ShapeNormalAlt
		}
	}
	if _, ok := cursor.Registered[shape]; !ok {
		shape = cursor.ShapeNormal
	}
	c.Cursors.Set(shape, now)
}

// componentCursor is TSe_Component's state, the pointing hand.
const componentCursor = 2
