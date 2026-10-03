// Wonderland patch: custom cursor images (see PATCHES.md).

package ebiten

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2/internal/ui"
)

// CursorImage is a custom mouse cursor: a picture and its hotspot.
type CursorImage struct {
	c *ui.CursorImage
}

// NewCursorImage makes a custom cursor from img with its hotspot at
// (hotX, hotY). The native cursor is created when first selected.
func NewCursorImage(img image.Image, hotX, hotY int) *CursorImage {
	return &CursorImage{c: &ui.CursorImage{Image: img, HotX: hotX, HotY: hotY}}
}

// SetCursorImage makes the system draw c as the mouse cursor while the
// cursor mode is visible; nil returns to the cursor shape (SetCursorShape).
// It is supported on Windows, macOS and Linux (X11, so also XWayland);
// elsewhere the cursor shape stays.
//
// SetCursorImage is concurrent-safe.
func SetCursorImage(c *CursorImage) {
	if c == nil {
		ui.Get().SetCursorImage(nil)
		return
	}
	ui.Get().SetCursorImage(c.c)
}
