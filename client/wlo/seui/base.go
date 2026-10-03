// Package seui ports the client's se_UI framework: TSe_GrBasic and its
// descendants, the form manager and the input handler. Addresses are from
// the WLRI build decompile (var/decompiled/aLogin_ca19ee087b60_full.c).
//
// Delphi virtual methods are the Control interface; each struct keeps a
// self reference so that base-class code calls overrides, as the VMT does.
// Virtual slot offsets are noted on each method.
package seui

import (
	"image"

	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
	"wonderland-go/client/wlo/text"
)

// Env holds the globals the framework reaches through PTR_DAT_ symbols: the
// picture database (PTR_DAT_004c9b94), the back buffer, the font
// (PTR_DAT_004ca63c), sound playback (FUN_00404fa4) and the form manager.
type Env struct {
	Pics   *picdb.DB
	Screen *surface.Surface
	Text   *text.Renderer
	Sound  func(path string)
	UI     *Manager
	// HintOwner is the control whose hint the main hint object shows
	// (PTR_DAT_004c9e14 +0x110); its own hint is not drawn.
	HintOwner Control
}

func (e *Env) play(path string) {
	if e != nil && e.Sound != nil {
		e.Sound(path)
	}
}

// Input is TSe_UIHandler (PTR_DAT_004c9db4): the pointer and the controls
// it relates to.
type Input struct {
	X, Y     int     // +0x04, +0x08
	Hovered  Control // +0x0c, recomputed by every frame's update pass
	Pressed  Control // +0x10
	Focused  Control // +0x14, set by a mouse press
	Captured Control // +0x18, a scroll button being dragged
}

// Control is the TSe_GrBasic VMT.
type Control interface {
	Base() *GrBasic
	SetSize(w, h int)   // +0x00
	SetColor(c uint16)  // +0x04
	AddChild(c Control) // +0x0c
	Paint()             // +0x10
	Tick()              // +0x14
	Update(in *Input)   // +0x18
	Hint()              // +0x1c
	Show()              // +0x20
	Hide()              // +0x24
	DblClick()          // +0x2c
	LeftUp(shift byte, x, y int)
	LeftUpOutside(shift byte, x, y int)
	LeftDown(shift byte, x, y int)
	RightUp(shift byte, x, y int)
	RightUpOutside(shift byte, x, y int)
	RightDown(shift byte, x, y int)
	MouseMove(shift byte, x, y int)
	CapturedMove(shift byte, x, y int)
	KeyDown(key uint16, shift byte) // +0x60
	KeyUp(key uint16, shift byte)   // +0x64
	HitTest(x, y int) bool          // +0x68
	Activate(on bool)               // +0x6c
}

// GrBasic is TSe_GrBasic (constructor FUN_00466488).
type GrBasic struct {
	self Control
	Env  *Env

	Name     string    // +0x04
	Parent   Control   // +0x08
	Children []Control // +0x0c, +0x10
	Visible  bool      // +0x14
	Focused  bool      // +0x15 the input handler's focused control
	Dragging bool      // +0x16
	Left     int       // +0x18
	Top      int       // +0x1c
	Width    int       // +0x20
	Height   int       // +0x24
	Color    uint16    // +0x28 ink set by SetColor; the skin colour
	Color2   uint16    // +0x2a
	Color3   uint16    // +0x2c an editor label's ink
	Clip     image.Rectangle
	Image    int  // +0x4c picture index, -1 for none
	UseClip  bool // +0xb4
	Inside   bool // +0xb5 under the pointer
	Stretch  bool // +0xb6
	PixelHit bool // +0xa8 hit tests use the image's pixels
	HasHint  bool // +0xa9
	HintText []byte
	Tag      int // +0xb0

	OnClick, OnDown, OnUp, OnRight func()        // +0x50, +0x58, +0x60, +0x68
	OnClickTag, OnDownTag          func(tag int) // +0x70, +0x78
	OnUpTag, OnRightTag, OnDblTag  func(tag int) // +0x80, +0x88, +0x90
	On98, OnA0                     func()        // +0x98, +0xa0
}

func (g *GrBasic) Base() *GrBasic { return g }

// initBasic is FUN_00466488 for an embedded GrBasic: the owner adopts the
// control through its AddChild slot.
func (g *GrBasic) initBasic(self Control, env *Env, owner Control) {
	g.self, g.Env = self, env
	g.Name = "Tse_GrBasic"
	if owner != nil {
		owner.AddChild(self)
	}
	g.Visible, g.Image, g.Color = true, -1, 0xffff
	g.PixelHit = true
}

// NewGrBasic creates a plain TSe_GrBasic.
func NewGrBasic(env *Env, owner Control) *GrBasic {
	g := &GrBasic{}
	g.initBasic(g, env, owner)
	return g
}

// Init is slot +0x08 (FUN_0046696c) with the original argument order:
// image name, left, clip height, clip width, clip top, clip left, use clip,
// height, width, top. Without a clip the whole control is the source.
func (g *GrBasic) Init(name string, left, clipH, clipW, clipY, clipX int, useClip bool, height, width, top int) {
	g.Image = g.Env.Pics.Find(name)
	g.Left, g.Top, g.Width, g.Height, g.UseClip = left, top, width, height, useClip
	if useClip {
		g.Clip = image.Rect(clipX, clipY, clipX+clipW, clipY+clipH)
	} else {
		g.Clip = image.Rect(0, 0, width, height)
	}
}

func (g *GrBasic) SetSize(w, h int)     { g.Width, g.Height = w, h }  // +0x00 FUN_00466a20
func (g *GrBasic) SetColor(c uint16)    { g.Color = c }               // +0x04 FUN_00466ef8
func (g *GrBasic) SetPos(x, y int)      { g.Left, g.Top = x, y }      // FUN_00466964
func (g *GrBasic) SetVisible(v bool)    { g.Visible = v }             // FUN_00466a1c
func (g *GrBasic) Tick()                {}                            // +0x14 FUN_00466e8c
func (g *GrBasic) Show()                { g.Visible = true }          // +0x20 FUN_004663d4
func (g *GrBasic) Hide()                { g.Visible = false }         // +0x24 FUN_00466594
func (g *GrBasic) Right() int           { return g.Left + g.Width }   // FUN_00466908
func (g *GrBasic) Bottom() int          { return g.Top + g.Height }   // FUN_0046686c
func (g *GrBasic) CenterX() int         { return g.Left + g.Width/2 } // FUN_00466e74
func (g *GrBasic) CenterY() int         { return g.Top + g.Height/2 } // FUN_00466e80
func (g *GrBasic) KeyDown(uint16, byte) {}
func (g *GrBasic) KeyUp(uint16, byte)   {}
func (g *GrBasic) Activate(bool)        {}

// AddChild is slot +0x0c (FUN_004663b8).
func (g *GrBasic) AddChild(c Control) {
	g.Children = append(g.Children, c)
	c.Base().Parent = g.self
}

// RemoveChild is FUN_00466928.
func (g *GrBasic) RemoveChild(c Control) {
	for i, k := range g.Children {
		if k == c {
			g.Children = append(g.Children[:i], g.Children[i+1:]...)
			c.Base().Parent = nil
			return
		}
	}
}

// SetHint is FUN_00466e90.
func (g *GrBasic) SetHint(s []byte) { g.HasHint, g.HintText = len(s) > 0, s }

// Root is FUN_00466914: the topmost ancestor.
func (g *GrBasic) Root() Control {
	if g.Parent != nil {
		return g.Parent.Base().Root()
	}
	return g.self
}

// ParentOrigin is FUN_00466e14: the sum of the ancestors' positions.
func (g *GrBasic) ParentOrigin() image.Point {
	if g.Parent == nil {
		return image.Point{}
	}
	p := g.Parent.Base()
	return p.ParentOrigin().Add(image.Pt(p.Left, p.Top))
}

// Abs is FUN_004668b4: the control's screen position.
func (g *GrBasic) Abs() image.Point {
	return g.ParentOrigin().Add(image.Pt(g.Left, g.Top))
}

// Rect is FUN_00466878: the control's screen rectangle.
func (g *GrBasic) Rect() image.Rectangle {
	a := g.Abs()
	return image.Rect(a.X, a.Y, a.X+g.Width, a.Y+g.Height)
}

// live reports the shared guard of the event slots: root and self visible.
func (g *GrBasic) live() bool { return g.Root().Base().Visible && g.Visible }

// HitTest is slot +0x68 (FUN_004663dc): inside the rectangle and, for
// pixel-tested images, on an opaque pixel.
func (g *GrBasic) HitTest(x, y int) bool {
	a := g.Abs()
	if !image.Pt(x, y).In(image.Rect(a.X, a.Y, a.X+g.Width, a.Y+g.Height)) {
		return false
	}
	if g.Image == -1 || !g.PixelHit {
		return true
	}
	return g.Env.Pics.Opaque(g.Image, x-a.X, y-a.Y)
}

// Paint is slot +0x10 (FUN_00466a28).
func (g *GrBasic) Paint() {
	if g.Image == -1 {
		return
	}
	a := g.Abs()
	switch {
	case g.Stretch:
		// FUN_0045ffb4 stretches the clip into a 32-pixel larger box; not
		// used by the login screens.
		g.Env.Pics.DrawRect(g.Env.Screen, g.Image, a.X, a.Y, g.Clip, true)
	case !g.UseClip:
		g.Env.Pics.Draw(g.Env.Screen, g.Image, a.X, a.Y, true)
	default:
		g.Env.Pics.DrawRect(g.Env.Screen, g.Image, a.X, a.Y, g.Clip, true)
	}
}

// Update is slot +0x18 (FUN_00466b70): hit test, paint, then the children
// from last to first, so earlier children are drawn on top.
func (g *GrBasic) Update(in *Input) {
	if !g.Visible {
		return
	}
	g.Inside = g.self.HitTest(in.X, in.Y)
	if g.Inside {
		in.Hovered = g.self
	}
	g.Focused = in.Focused == g.self
	g.self.Paint()
	g.updateChildren(in)
}

func (g *GrBasic) updateChildren(in *Input) {
	for i := len(g.Children) - 1; i >= 0; i-- {
		g.Children[i].Update(in)
	}
}

// Hint is slot +0x1c (FUN_00466be4): a tooltip above the control. The box
// is filled with $F98B3D at alpha 200 (FillRectAlpha) and framed by a
// one-pixel $800000 pen with a clear brush; the text is drawn by the font
// in style 2, white over a 0x0841 shadow.
func (g *GrBasic) Hint() {
	if !g.Visible || !g.HasHint || len(g.HintText) == 0 {
		return
	}
	a := g.Abs()
	r := image.Rect(a.X-1, a.Y-0x1a, a.X-1+len(g.HintText)*8+7, a.Y-0x1a+0x17)
	dx, dy := 0, 0
	if r.Max.X > 800 {
		dx = 0x31e - r.Max.X
		r = r.Add(image.Pt(dx, 0))
	}
	if r.Min.X < 0 {
		dx = 2 - r.Min.X
		r = r.Add(image.Pt(dx, 0))
	}
	if r.Min.Y < 0 {
		dy = 0x32
		r = r.Add(image.Pt(0, dy))
	}
	g.Env.Screen.FillAlpha(r, hintFill, hintAlpha)
	g.Env.Screen.Frame(r, surface.TColor(hintPen))
	g.Env.Text.Draw(a.X+2+dx, a.Y-0x16+dy, 0, false, true, g.Env.Screen, g.HintText, 0x10, 400, hintShadow, hintInk, 2)
}

// Hint box colours (FUN_00466be4's arguments).
const (
	hintFill   = 0xf98b3d // TColor
	hintAlpha  = 200
	hintPen    = 0x800000 // TColor: navy
	hintShadow = 0x0841
	hintInk    = 0xffff
)

// Event slots. Each fires its handlers only while the control and its root
// are visible.

func (g *GrBasic) DblClick() { // +0x2c FUN_004665cc
	if g.live() && g.OnDblTag != nil {
		g.OnDblTag(g.Tag)
	}
}

func (g *GrBasic) LeftUp(byte, int, int) { // +0x30 FUN_00466674
	if !g.live() {
		return
	}
	if g.OnClick != nil {
		g.OnClick()
	}
	if g.OnClickTag != nil {
		g.OnClickTag(g.Tag)
	}
}

func (g *GrBasic) LeftUpOutside(byte, int, int) {} // +0x34

func (g *GrBasic) LeftDown(byte, int, int) { // +0x38 FUN_00466634
	if !g.live() {
		return
	}
	if g.OnDown != nil {
		g.OnDown()
	}
	if g.OnDownTag != nil {
		g.OnDownTag(g.Tag)
	}
}

func (g *GrBasic) RightUp(byte, int, int) { // +0x3c FUN_00466800
	if !g.live() {
		return
	}
	if g.OnUp != nil {
		g.OnUp()
	}
	if g.OnUpTag != nil {
		g.OnUpTag(g.Tag)
	}
}

func (g *GrBasic) RightUpOutside(byte, int, int) {} // +0x40

func (g *GrBasic) RightDown(byte, int, int) { // +0x44 FUN_004667b4
	if !g.live() {
		return
	}
	if g.OnRight != nil {
		g.OnRight()
	}
	if g.OnRightTag != nil {
		g.OnRightTag(g.Tag)
	}
}

func (g *GrBasic) MouseMove(byte, int, int)    {} // +0x48
func (g *GrBasic) CapturedMove(byte, int, int) {} // +0x4c

// InitGrBasic initialises a GrBasic embedded in a control defined outside
// this package (FUN_00466488).
func InitGrBasic(g *GrBasic, self Control, env *Env, owner Control) {
	g.initBasic(self, env, owner)
}
