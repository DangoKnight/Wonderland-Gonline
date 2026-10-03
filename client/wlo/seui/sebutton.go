package seui

import (
	"image"
	"strconv"
)

// Button is TSe_Button (constructor FUN_00467834): a nine-slice panel whose
// image row is the button state (+0x118) rather than the panel's hover
// state, with an optional icon of one or three rows.
type Button struct {
	Panel
	BState    byte // +0x118: 0 idle, 1 under the pointer, 2 held
	FontSize  byte // +0x119
	Held      bool // +0x11a pressed and not yet released
	IconRows  bool // +0x11b the icon has a row per state
	IconDown  bool // +0x124 the icon always shows its third row
	Sticky    bool // +0x125 stays in the held state
	TextStyle byte // +0x126
	CharW     int  // +0x11c
	CharH     int  // +0x120
	Caption   []byte
	TextX     int // +0x40
	TextY     int // +0x44
}

func NewButton(env *Env, owner Control) *Button {
	b := &Button{}
	b.initPanel(b, env, owner)
	b.SetMargins(4, 4, 4, 4)
	b.Name = "button"
	b.setFontSize(0)
	b.IconDown = false
	b.TextStyle = 2
	return b
}

// setFontSize is FUN_004679f8.
func (b *Button) setFontSize(size byte) {
	b.FontSize = size
	switch size {
	case 0:
		b.CharW, b.CharH = 8, 0xf
	case 1:
		b.CharW, b.CharH = 10, 0x13
	case 2:
		b.CharW, b.CharH = 0xc, 0x18
	}
}

// SetCaption is FUN_004679a0 with FUN_004679bc's centring.
func (b *Button) SetCaption(s []byte) {
	b.Caption = s
	b.TextY = (b.Height-b.CharH)/2 + b.Top
	b.TextX = (b.Width-len(s)*b.CharW)/2 + b.Left
}

// SetStateIcon is FUN_00467a4c: the panel icon, with rows per state.
func (b *Button) SetStateIcon(name string, x, h, w, y int, rows bool) {
	b.SetIcon(name, x, h, w, y)
	b.IconRows = rows
}

// LeftDown is slot +0x38 (FUN_004678bc).
func (b *Button) LeftDown(shift byte, x, y int) {
	if !b.live() || !b.Enabled {
		return
	}
	b.Component.LeftDown(shift, x, y)
	if !b.Blocked() && b.self.HitTest(x, y) {
		b.BState, b.Held = 2, true
	}
}

// LeftUp is slot +0x30 (FUN_0046792c): a release after a press clicks.
func (b *Button) LeftUp(shift byte, x, y int) {
	if !b.live() || !b.Enabled || b.Blocked() || !b.Held {
		return
	}
	b.Component.LeftUp(shift, x, y)
	b.BState = 1
	if b.Sticky {
		b.BState = 2
	}
}

// Update is slot +0x18 (FUN_00467abc): the panel's update, then the state
// follows the pointer.
func (b *Button) Update(in *Input) {
	b.Panel.Update(in)
	if !b.Enabled {
		return
	}
	if in.Hovered == b.self && !b.Blocked() {
		if b.BState == 2 {
			return
		}
		b.BState = 1
		if b.Sticky {
			b.BState = 2
		}
		return
	}
	b.BState = 0
	if b.Sticky {
		b.BState = 2
	}
	b.Held = false
}

// Paint is slot +0x10 (FUN_00467ab4 → FUN_0046ae40) with the button's
// slice drawers (+0x7c..+0x90), which take the row from BState, and its
// icon drawer (+0x78, FUN_00467e80).
func (b *Button) Paint() {
	o := b.ParentOrigin()
	if b.Image != -1 {
		h := b.Clip.Dy()
		row := int(b.BState)*h + b.Clip.Min.Y
		clip := image.Rect(b.Clip.Min.X, row, b.Clip.Max.X, row+h)
		b.ninePatch(image.Rect(o.X+b.Left, o.Y+b.Top, o.X+b.Left+b.Width, o.Y+b.Top+b.Height), clip)
	}
	// The centre slice's drawer (FUN_004680d8) ends with the caption,
	// recentred (FUN_004679bc), in the button's colours and style.
	if len(b.Caption) > 0 {
		b.SetCaption(b.Caption)
		b.Env.Text.Draw(o.X+b.TextX, o.Y+b.TextY, 0, false, true, b.Env.Screen, b.Caption, b.CharH, b.Width, b.Color2, b.Color, b.TextStyle)
	}
	if b.Icon == -1 {
		return
	}
	y := b.IconY
	switch {
	case !b.IconRows:
	case !b.IconDown:
		y += int(b.BState) * b.IconH
	default:
		y += 2 * b.IconH
	}
	r := image.Rect(b.IconX, y, b.IconX+b.IconW, y+b.IconH)
	x := o.X + b.CenterX() - int(uint32(b.IconW)>>1)
	yy := o.Y + b.CenterY() - int(uint32(b.IconH)>>1)
	b.Env.Pics.DrawRect(b.Env.Screen, b.Icon, x, yy, r, true)
}

// ElementIcon is TRe_ElementIconPanel (constructor FUN_00479290): a panel
// showing an element's icon with its name as the hint.
type ElementIcon struct{ Panel }

// elementNames are the hints at DAT_00479488.
var elementNames = []string{"None", "Eart", "Watr", "Fire", "Wind"}

func NewElementIcon(env *Env, owner Control) *ElementIcon {
	e := &ElementIcon{}
	e.initPanel(e, env, owner)
	return e
}

// SetElement is FUN_004792f0: Icon_Element_<n>_1 at the given position
// (-1 keeps the current one) with the element's name as the hint.
func (e *ElementIcon) SetElement(elem byte, left int, hover bool, cursor byte, hint []byte, height, width, top int) {
	if hint == nil && int(elem) < len(elementNames) {
		hint = []byte(elementNames[elem])
	}
	if left == -1 {
		left = e.Left
	}
	if top == -1 {
		top = e.Top
	}
	if width == -1 {
		width = e.Width
	}
	if height == -1 {
		height = e.Height
	}
	e.Init("Icon_Element_"+itoa(int(elem))+"_1", left, 0, 0, 0, 0, false, height, width, top)
	e.Cursor = cursor
	e.Hover = hover
	e.SetHint(hint)
}

func itoa(n int) string { return strconv.Itoa(n) }
