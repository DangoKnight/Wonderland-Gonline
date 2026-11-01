package hud

import (
	"strconv"

	"wonderland-gonline/client/wlo/seui"
)

// MiniMap is the map frame's minimap (constructor FUN_0030e100, a TSe_Form
// that cannot be dragged, +0xbb = 1; setup FUN_0030e1b0, paint
// FUN_0030e978): the scene's picture <scene>_Mini_Form at the top right,
// 0x26 from the top (the picture is named by the map ID), with Reload and Minimize buttons along its top edge.
// Minimized (FUN_0030e5bc) it becomes the small Form_HotKey_Mini with an
// Expand button. The application draws the marks (Marks) while it is
// expanded.
type MiniMap struct {
	seui.Form
	Minimize *seui.FixedButton // +0x134
	Reload   *seui.FixedButton // +0x138
	// Scale is the map width over the picture's (+0x140); 0 draws no
	// marks.
	Scale int
	// Minimized is +0x142.
	Minimized bool
	// Map is the player's map (+0x235c), whose ID names the picture;
	// MapWidth is the map's width.
	Map         uint16
	MapWidth    int
	shown       bool
	ScreenWidth int
	LoadPicture func(name string)
	Marks       func(m *MiniMap)
}

const (
	miniMapSuffix = "_Mini_Form"
	miniMapTop    = 0x26
	// Maps 10031..10040 show 10003's picture (FUN_0030e1b0); expanding
	// again maps 11031..11040 to 11031 instead (FUN_0030e5bc).
	miniMapAliasFirst   = 10031
	miniMapAlias        = 10003
	miniMapExpandFirst  = 11031
	miniMapAliasCount   = 10
	miniMapMinimized    = "Form_HotKey_Mini"
	miniMapMinWidth     = 0x27
	miniMapMinHeight    = 0x26
	miniMapExpand       = "Btn_Window_1"
	miniMapExpandLeft   = 10
	miniMapExpandTop    = 10
	miniMapButtonWidth  = 0x13
	miniMapButtonHeight = 0x12
	// MiniMapSkipMap (0xea6c) never shows a minimap.
	MiniMapSkipMap = 0xea6c
)

// NewMiniMap is FUN_0030e100: the form and its buttons, hidden.
func NewMiniMap(env *seui.Env, screenW int) *MiniMap {
	m := &MiniMap{ScreenWidth: screenW}
	m.InitForm(m, env)
	m.Name = "CH_TMiniMapForm_World"
	m.Draggable = false
	m.Minimize = newBarButton(env, m, "Btn_Minimize_1", 0, 0, "Minimize Map")
	m.Minimize.OnClick = m.Toggle
	m.Reload = newBarButton(env, m, "Btn_Reload", 0, 0, "Reload Mini-map")
	m.Reload.OnClick = func() { m.Setup(m.Map, m.MapWidth) }
	return m
}

// pictureName is a map's picture.
func pictureName(id uint16) string {
	return strconv.Itoa(int(id)) + miniMapSuffix
}

// find looks the picture up, loading it on first use (FUN_004623f0).
func (m *MiniMap) find(name string) int {
	i := m.Env.Pics.Find(name)
	if i < 0 && m.LoadPicture != nil {
		m.LoadPicture(name)
		i = m.Env.Pics.Find(name)
	}
	return i
}

// Setup is FUN_0030e1b0 for a map and its width: the form takes the
// picture's size at the right edge, or hides without one.
func (m *MiniMap) Setup(id uint16, mapWidth int) {
	m.Map, m.MapWidth, m.Minimized = id, mapWidth, false
	m.Scale, m.shown = 0, false
	if id == MiniMapSkipMap {
		return
	}
	if id >= miniMapAliasFirst && id < miniMapAliasFirst+miniMapAliasCount {
		id = miniMapAlias
	}
	name := pictureName(id)
	i := m.find(name)
	if i < 0 {
		m.Hide()
		return
	}
	w, h := m.Env.Pics.Size(i)
	m.Init(name, m.ScreenWidth-w, h, w, 0, 0, false, h, w, miniMapTop)
	m.Scale = mapWidth / w
	m.shown = true
	bw, bh := m.buttonSize("Btn_Minimize_1")
	m.Minimize.Init("Btn_Minimize_1", w-bw, bh, bw, 0, 0, true, bh, bw, 0)
	m.Minimize.SetHint([]byte("Minimize Map"))
	m.Minimize.SetVisible(true)
	rw, rh := m.buttonSize("Btn_Reload")
	m.Reload.Init("Btn_Reload", w-2*rw, rh, rw, 0, 0, true, rh, rw, 0)
	m.Reload.SetVisible(true)
	m.Show()
}

// Shown reports whether the map has a minimap (+0x143).
func (m *MiniMap) Shown() bool { return m.shown }

// buttonSize is a three-row button picture's width and row height.
func (m *MiniMap) buttonSize(name string) (int, int) {
	w, h := m.Env.Pics.Size(m.Env.Pics.Find(name))
	return w, h / stateRows
}

// Toggle is FUN_0030e5bc: minimize to the small form centred on the
// Minimize button, or expand back so that the button stays put.
func (m *MiniMap) Toggle() {
	b := m.Minimize
	if !m.Minimized {
		left := m.Left + b.Left - (miniMapMinWidth-b.Width)/2
		top := m.Top + b.Top - (miniMapMinHeight-b.Height)/2
		m.Init(miniMapMinimized, left, 0, 0, 0, 0, false, miniMapMinHeight, miniMapMinWidth, top)
		b.Init(miniMapExpand, miniMapExpandLeft, miniMapButtonHeight, miniMapButtonWidth, 0, 0, true, miniMapButtonHeight, miniMapButtonWidth, miniMapExpandTop)
		b.SetHint([]byte("Expand Map"))
		m.Minimized = true
		m.Reload.SetVisible(false)
		return
	}
	id := m.Map
	if id >= miniMapExpandFirst && id < miniMapExpandFirst+miniMapAliasCount {
		id = miniMapExpandFirst
	}
	name := pictureName(id)
	i := m.find(name)
	if i < 0 {
		m.Scale, m.shown = 0, false
		m.Hide()
		return
	}
	w, h := m.Env.Pics.Size(i)
	left := m.Left + b.Right() - w
	top := m.Top + b.Top
	m.Init(name, left, h, w, 0, 0, true, h, w, top)
	b.Init("Btn_Minimize_1", w-miniMapButtonWidth, miniMapButtonHeight, miniMapButtonWidth, 0, 0, true, miniMapButtonHeight, miniMapButtonWidth, 0)
	b.SetHint([]byte("Minimize map"))
	m.Minimized = false
	m.Reload.SetVisible(true)
}

// Paint is FUN_0030e978: the picture, then the marks (FUN_0030f27c and
// FUN_0030eb60) while expanded.
func (m *MiniMap) Paint() {
	m.Form.Paint()
	if !m.Minimized && m.Scale != 0 && m.Marks != nil {
		m.Marks(m)
	}
}
