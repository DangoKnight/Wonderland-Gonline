// Wonderland patch: custom cursor images (see PATCHES.md).

package ui

import (
	"image"
	"sync/atomic"
)

// CursorImage is a custom cursor picture with its hotspot. Its native
// cursor is made on first use by a backend that supports it.
type CursorImage struct {
	Image      image.Image
	HotX, HotY int

	native any
}

// customCursor is the cursor image in use, nil for the cursor shape.
var customCursor atomic.Pointer[CursorImage]

// SetCursorImage selects a custom cursor image; nil returns to the cursor
// shape. Backends without custom cursors keep the shape.
func (u *UserInterface) SetCursorImage(c *CursorImage) {
	if customCursor.Swap(c) == c {
		return
	}
	u.applyCustomCursor()
}
