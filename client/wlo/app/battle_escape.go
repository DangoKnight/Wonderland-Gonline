package app

import (
	"image"
	"wonderland-gonline/client/wlo/surface"
)

// Native MBTM 60046: three running frames followed by the 26/27 falling
// pair. 60042 uses the 64/65 running pair and is not a failed escape.
const battleFleeFallFrame = 4
const battleFailInk = 0xf800
const battleFailOutline = 0xffe0
const battleFailScale = 2

func (c *Client) drawEscapeFailure(at image.Point) {
	label := surface.New(36, 20)
	defer label.Close()
	for _, offset := range []image.Point{{0, 1}, {2, 1}, {1, 0}, {1, 2}} {
		c.Env.Text.Draw(offset.X, offset.Y, 0, false, true, label, []byte("Fail"), 18, 36, 0, battleFailOutline, 0)
	}
	c.Env.Text.Draw(1, 1, 0, false, true, label, []byte("Fail"), 18, 36, 0, battleFailInk, 0)
	x, y := at.X-18*battleFailScale, at.Y-battleFloatOffsetPixels
	c.Screen.DrawStretch(image.Rect(x, y, x+36*battleFailScale, y+20*battleFailScale), label, true)
}
