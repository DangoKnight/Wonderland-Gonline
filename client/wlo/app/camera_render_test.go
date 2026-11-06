package app

import (
	"image"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

func TestSubpixelCameraKeepsActorsAlignedWithScenery(t *testing.T) {
	c, now, _ := enteredClient(t)
	w := c.World
	w.Body, w.NPCs, w.Peers, w.Companions, w.Weather = nil, nil, nil, nil, nil
	w.Cinematic, w.HideNames = true, true
	w.Step(*now)
	w.Player.X += 3
	*now = now.Add(16 * time.Millisecond)
	w.Step(*now)
	x, y := w.Player.X, w.Player.Y
	// Identical white shapes in the terrain and actor passes must reconstruct
	// identically. Separate integer/fractional cameras make their edges diverge.
	background := surface.New(40, 32)
	background.Fill(image.Rect(8, 8, 12, 12), 0xffff)
	w.Scene.Layers = []world.Layer{{X: x - 8, Y: y - 16, Image: background}}
	foreground := surface.New(4, 2)
	foreground.Fill(image.Rect(0, 0, 4, 2), 0x07e0)
	foreground.Pix[1] = 0
	w.Scene.Objects = []world.Object{{X: x + 12, Y: y, Depth: 2, Kind: world.ObjectSorted, Frames: 1, Image: foreground}}
	sprites := []world.SceneSprite{{X: x, Y: y, Draw: func(dst *surface.Surface, x, y int) {
		dst.Fill(image.Rect(x, y, x+4, y+4), 0xffff)
		dst.Fill(image.Rect(x+12, y, x+16, y+4), 0xffff)
	}}}
	if !c.drawSubpixelWorld(sprites) {
		t.Fatal("fixture did not use fractional camera")
	}
	cx, cy := w.Camera()
	intermediate := false
	for row := -1; row <= 4; row++ {
		for col := -1; col <= 4; col++ {
			actor := c.Screen.Pix[(y-cy+row)*ScreenWidth+x-cx+col]
			terrain := c.Screen.Pix[(y-cy+row-8)*ScreenWidth+x-cx+col]
			if actor != terrain {
				t.Fatalf("actor and terrain shifted differently at %d,%d: %04x vs %04x", col, row, actor, terrain)
			}
			intermediate = intermediate || actor != 0 && actor != 0xffff
		}
	}
	if !intermediate {
		t.Fatal("fractional motion reverted to whole-pixel stepping")
	}
	covered := surface.Expand(c.Screen.Pix[(y-cy)*ScreenWidth+x-cx+14])
	if covered.G <= covered.R || covered.G <= covered.B {
		t.Fatal("foreground no longer occludes actor")
	}
	if w.HideScene || w.CameraAt != nil || w.DeferOverlays || c.Env.Screen != c.Screen {
		t.Fatal("camera pass leaked scene state")
	}
}
