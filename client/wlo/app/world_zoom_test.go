package app

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestWorldZoomLimitsAndUIExclusion(t *testing.T) {
	if !worldZoomEnabled {
		t.Skip("wheel zoom temporarily disabled")
	}
	c, _, _ := enteredClient(t)
	c.Input.X, c.Input.Y = 650, 300
	c.mapReady, c.held = true, false
	c.Input.Hovered = nil
	c.uiHovered = false
	for range 20 {
		if !c.zoomWorld(false) {
			t.Fatal("map zoom rejected")
		}
	}
	if c.worldZoom.level != worldZoomMinimum || c.World.ViewWidth <= ScreenWidth || c.World.ViewHeight <= ScreenHeight {
		t.Fatal("zoom out did not expose a larger viewport")
	}
	for range 30 {
		c.zoomWorld(true)
	}
	if c.worldZoom.level != worldZoomMaximum || c.World.ViewWidth != 640 || c.World.ViewHeight != 480 {
		t.Fatal("zoom-in limit incorrect")
	}
	before := c.worldZoom.level
	c.Inventory.Visible = true
	r := c.Inventory.Rect()
	c.Input.X, c.Input.Y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	c.Input.Hovered = nil // No paint has updated hover since moving onto the form.
	if c.zoomWorld(false) || c.worldZoom.level != before {
		t.Fatal("wheel over a control zoomed map")
	}
	c.Input.Hovered = nil
	c.Inventory.Visible = false
	c.Input.X, c.Input.Y = 650, 300
	c.UI.Modal = c.Inventory
	if c.zoomWorld(false) {
		t.Fatal("wheel escaped a modal")
	}
	c.UI.Modal = nil
	c.Input.X = -1
	if c.zoomWorld(false) {
		t.Fatal("wheel outside game zoomed map")
	}
	c.Input.X = 650
	c.battle.state.Active = true
	if c.zoomWorld(false) {
		t.Fatal("wheel changed scripted battle camera")
	}
}

func TestWorldZoomRenderingRestoresUIAndPicking(t *testing.T) {
	if !worldZoomEnabled {
		t.Skip("wheel zoom temporarily disabled")
	}
	c, _, _ := enteredClient(t)
	for _, level := range []int{worldZoomMinimum, worldZoomMaximum, worldZoomDefault} {
		c.worldZoom.level = level
		c.syncWorldZoom()
		cx, cy := c.World.Camera()
		var hit bool
		for _, n := range c.World.NPCs {
			if !n.Visible() || n.Painter == nil {
				continue
			}
			r := n.Painter.Bounds(n.X-cx, n.Y-cy+n.Info.SpriteDrop(), n.Action)
			pt := r.Min.Add(image.Pt(r.Dx()/2, r.Dy()/2))
			if c.World.NPCAt(pt.X, pt.Y) == nil {
				continue
			}
			w, h := c.World.ViewWidth, c.World.ViewHeight
			if w == 0 {
				w, h = ScreenWidth, ScreenHeight
			}
			sx, sy := pt.X*ScreenWidth/w, pt.Y*ScreenHeight/h
			x, y := c.World.ScenePoint(sx, sy)
			if c.World.NPCAt(x, y) == nil {
				t.Fatal("zoomed NPC pick missed visible character")
			}
			hit = true
			break
		}
		if !hit {
			t.Fatal("no NPC available for zoom pick test")
		}
		screen := c.Screen
		c.drawWorldScene()
		if c.Screen != screen || c.Env.Screen != screen || c.World.Env.Screen != screen {
			t.Fatal("scene target leaked into UI")
		}
		if dir := os.Getenv("WORLD_ZOOM_SNAPSHOT_DIR"); dir != "" {
			c.UI.Draw()
			savePNG(t, filepath.Join(dir, string(rune('a'+level-worldZoomMinimum))+".png"), c)
		}
	}
	c.closeWorldZoom()
	if c.worldZoom.canvas != nil {
		t.Fatal("zoom canvas retained on close")
	}
}

func TestWorldZoomDisabledUsesStandardViewport(t *testing.T) {
	if worldZoomEnabled {
		t.Skip("wheel zoom enabled")
	}
	c, _, _ := enteredClient(t)
	c.worldZoom.level = worldZoomMinimum
	c.World.ViewWidth, c.World.ViewHeight = 1067, 800
	c.syncWorldZoom()
	if c.worldZoom.level != worldZoomDefault || c.World.ViewWidth != 0 || c.World.ViewHeight != 0 {
		t.Fatal("old zoom persisted")
	}
	if c.zoomWorld(true) || c.zoomWorld(false) || c.worldZoom.level != worldZoomDefault {
		t.Fatal("wheel changed disabled zoom")
	}
}
