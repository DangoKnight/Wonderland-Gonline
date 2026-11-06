package app

import (
	"strconv"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

// The minimap (hud.MiniMap). Entering a map sets it up for the player's
// map when its SceneData record's +0x1d is 1 (FUN_003f830c, 0x3f8dcd),
// otherwise hides it; it hides with the HUD during battles, movies and
// minigames (the HUD visibility checks around FUN_004a1d60).
const (
	// miniMapFaceAction is the marker's face action (+0x121 = 4,
	// FUN_0030eb60), drawn on frame 0 (+0x11e is reset every paint).
	miniMapFaceAction = 4
)

func (c *Client) newMiniMap() {
	c.MiniMap = hud.NewMiniMap(c.Env, ScreenWidth)
	c.MiniMap.LoadPicture = func(name string) { c.loadPictures(name) }
	c.MiniMap.Marks = c.drawMiniMapMarks
	c.UI.Add(c.MiniMap)
}

// setupMiniMap is the map load's minimap step.
func (c *Client) setupMiniMap() {
	if c.MiniMap == nil || c.World == nil || c.World.Scene == nil {
		return
	}
	if c.resources.sceneMiniMaps == nil {
		c.resources.sceneMiniMaps, _ = world.SceneMiniMaps(c.Assets)
	}
	id := c.World.Player.Map
	if !c.resources.sceneMiniMaps[id] {
		c.MiniMap.Setup(hud.MiniMapSkipMap, 0)
		c.MiniMap.Hide()
		return
	}
	c.MiniMap.Setup(id, c.World.Scene.Width)
	c.syncMiniMap()
}

// syncMiniMap shows the minimap only with the map's HUD.
func (c *Client) syncMiniMap() {
	m := c.MiniMap
	if m == nil {
		return
	}
	want := m.Shown() && c.World != nil && c.G.InGame && !c.battle.state.Active && c.movie == nil && c.sport == nil
	switch {
	case want && !m.Visible:
		m.Show()
	case !want && m.Visible:
		m.Hide()
	}
}

// drawMiniMapMarks is FUN_0030f27c (signs) then FUN_0030eb60 (the player).
func (c *Client) drawMiniMapMarks(m *hud.MiniMap) {
	w := c.World
	if w == nil || w.Scene == nil {
		return
	}
	at := m.Abs()
	point := func(x, y int) (int, int) {
		return world.MiniMapPoint(x, y, m.Scale, at.X, at.Y, w.Scene.Height)
	}
	pics := c.Pics
	for _, s := range w.MiniMapSigns() {
		i := pics.Find(world.SignPicture + strconv.Itoa(int(s.Sign)))
		if i < 0 {
			continue
		}
		iw, ih := pics.Size(i)
		x, y := point(s.At.X, s.At.Y)
		pics.Draw(c.Screen, i, x-iw/2, y-ih/2, true)
	}
	p := w.Player
	if login.RoleIndex(byte(p.Body), byte(p.Head)) == 0 {
		return
	}
	if f, ok := w.Body.(interface {
		DrawFaceFrame(dst *surface.Surface, x, y, action, frame int)
	}); ok {
		x, y := point(p.X, p.Y)
		f.DrawFaceFrame(c.Screen, x, y, miniMapFaceAction, 0)
	}
}
