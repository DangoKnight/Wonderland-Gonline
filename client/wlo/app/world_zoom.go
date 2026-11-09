package app

import (
	"image"
	"math"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

const (
	worldZoomEnabled = false // Temporarily disabled while evaluating scaling filters.
	worldZoomDefault = 20
	worldZoomMinimum = 15 // 0.75x
	worldZoomMaximum = 25 // 1.25x
)

type worldZoomState struct {
	level  int
	canvas *surface.Surface
}

func (c *Client) syncWorldZoom() {
	if c.World == nil {
		return
	}
	if !worldZoomEnabled {
		c.worldZoom.level = worldZoomDefault
	}
	level := c.worldZoom.level
	if level == 0 {
		level = worldZoomDefault
	}
	if level == worldZoomDefault {
		c.World.ViewWidth, c.World.ViewHeight = 0, 0
		return
	}
	c.World.ViewWidth = (ScreenWidth*worldZoomDefault + level - 1) / level
	c.World.ViewHeight = (ScreenHeight*worldZoomDefault + level - 1) / level
}
func (c *Client) zoomWorld(up bool) bool {
	if !worldZoomEnabled {
		c.syncWorldZoom()
		return false
	}
	if c.World == nil || !c.mapReady || c.battle.state.Active || c.movie != nil || c.sport != nil || c.constellation != nil || c.sceneFrozen() || !c.clinicUntil.IsZero() || c.held || c.event.active || c.UI.Modal != nil || c.zoomPointerOverUI() || c.Talk.Contains(c.Input.X, c.Input.Y) {
		return false
	}
	if !image.Pt(c.Input.X, c.Input.Y).In(image.Rect(0, 0, ScreenWidth, ScreenHeight)) {
		return false
	}
	if c.worldZoom.level == 0 {
		c.worldZoom.level = worldZoomDefault
	}
	if up {
		c.worldZoom.level = min(worldZoomMaximum, c.worldZoom.level+1)
	} else {
		c.worldZoom.level = max(worldZoomMinimum, c.worldZoom.level-1)
	}
	c.syncWorldZoom()
	return true
}

func (c *Client) zoomPointerOverUI() bool {
	if c.Input.Captured != nil {
		return true
	}
	// Hit-test the current pointer, rather than the last painted hover state:
	// moving over a panel and scrolling can occur in the same update.
	var hit func(seui.Control) bool
	hit = func(control seui.Control) bool {
		if !control.Base().Visible {
			return false
		}
		if control.HitTest(c.Input.X, c.Input.Y) {
			return true
		}
		for _, child := range control.Base().Children {
			if hit(child) {
				return true
			}
		}
		return false
	}
	for _, control := range c.UI.Forms {
		if hit(control) {
			return true
		}
	}
	return false
}
func (c *Client) closeWorldZoom() {
	if c.worldZoom.canvas != nil {
		c.worldZoom.canvas.Close()
	}
	c.worldZoom = worldZoomState{}
	c.syncWorldZoom()
}

func (c *Client) drawWorldScene() {
	c.syncWorldZoom()
	sprites := append(c.floorItemSprites(), c.placedVehicleSprites()...)
	sprites = append(sprites, c.tentSceneSprites()...)
	if c.World.ViewWidth == 0 {
		if c.drawSubpixelWorld(sprites) {
			return
		}
		c.World.DrawWithSprites(sprites)
		c.drawWorldItemGains(c.Now())
		c.drawVehicleEffects()
		return
	}
	width, height := c.World.ViewWidth, c.World.ViewHeight
	canvas := c.worldZoom.canvas
	if canvas == nil || canvas.W != width || canvas.H != height {
		if canvas != nil {
			canvas.Close()
		}
		canvas = c.Screen.NewCompatible(width, height)
		c.worldZoom.canvas = canvas
	}
	canvas.Fill(image.Rect(0, 0, width, height), 0)
	// World painters share the environment but receive the scene target only
	// during this pass. HUD/dialogues are painted afterward on the native UI.
	screen := c.Screen
	c.Screen, c.Env.Screen = canvas, canvas
	c.World.DrawWithSprites(sprites)
	c.drawWorldItemGains(c.Now())
	c.drawVehicleEffects()
	c.Screen, c.Env.Screen = screen, screen
	screen.DrawStretch(image.Rect(0, 0, ScreenWidth, ScreenHeight), canvas, false)
	c.World.DrawLocation()
}

// Reconstruct the whole scene with one camera transform. Splitting background
// and characters between fractional and integer cameras causes relative jitter.
func (c *Client) drawSubpixelWorld(sprites []world.SceneSprite) bool {
	w := c.World
	if w.HideScene {
		return false
	}
	x, y := w.CameraPosition()
	ix, iy := math.Floor(x), math.Floor(y)
	fx, fy := x-ix, y-iy
	if fx < 1.0/256 && fy < 1.0/256 {
		return false
	}
	const border = 2 // Four-tap reconstruction needs one extra pixel on each side.
	width, height := ScreenWidth+border*2, ScreenHeight+border*2
	canvas := c.worldZoom.canvas
	if canvas == nil || canvas.W != width || canvas.H != height {
		if canvas != nil {
			canvas.Close()
		}
		canvas = c.Screen.NewCompatible(width, height)
		c.worldZoom.canvas = canvas
	}
	canvas.Fill(image.Rect(0, 0, width, height), 0)
	origin := image.Pt(int(ix)-border, int(iy)-border)
	screen, oldCamera, oldOverlay := c.Screen, w.CameraAt, w.DeferOverlays
	c.Screen, c.Env.Screen = canvas, canvas
	w.CameraAt, w.DeferOverlays = &origin, true
	w.DrawWithSprites(sprites)
	c.drawWorldItemGains(c.Now())
	c.drawVehicleEffects()
	c.Screen, c.Env.Screen = screen, screen
	w.CameraAt, w.DeferOverlays = oldCamera, oldOverlay
	screen.DrawSubpixel(image.Rect(0, 0, ScreenWidth, ScreenHeight), canvas, border+fx, border+fy)
	w.DrawOverlays()
	return true
}
