package seui

import (
	"image"
	"strconv"

	"wonderland-go/client/wlo/surface"
)

// SelectText is TSe_SelectText (constructor FUN_0046ec9c): a list of text
// rows with hover and selection highlights.
type SelectText struct {
	Component
	Items       [][]byte // +0x12c
	Colors      []uint16 // +0x138 ink per item, taken from Color when added
	Icons       []string // +0x14c picture index per item, "-1" for none
	Data        []string // +0x150
	HoverRow    int      // +0xd4 visible row under the pointer, -1
	TopIndex    int      // +0xd8
	HoverRow5   int      // +0xdc
	RowsShown   int      // +0x118
	LineHeight  int      // +0x11c
	RowHeight   int      // +0x120
	Spacing     int      // +0x144
	TextX       int      // +0x148
	IconY       int      // +0x158
	TextY       int      // +0x154
	Scroll      *ScrollButton
	FocusPen    uint32 // +0x130 TColor
	Highlight   uint32 // +0x134 TColor
	Embossed    bool   // +0x13c text style 2
	FocusBox    bool   // +0x13d
	StickHover  bool   // +0x13e hover stays on the last row
	Selected    int    // +0x140
	ShowSel     bool   // +0x142
	Untoggle    bool   // +0x143 clicking the selection clears it
	BlendHover  bool   // +0x15c
	HoverAlpha  byte   // +0x15d
	BlendSel    bool   // +0x15e
	SelAlpha    byte   // +0x15f
	ShowHover   bool   // +0x160
	ShowIcons   bool   // +0x161
	HighImg     int    // +0x164
	FocusAlways bool   // +0x168

	OnSelect   func(i int) // +0xe0 click or Enter
	OnRightUp  func(i int) // +0xe8
	OnDblClick func(i int) // +0xf0
	OnSlot28   func(i int) // +0xf8
	OnMove     func(i int) // +0x100
	OnScroll   func()      // +0x108
}

func NewSelectText(env *Env, owner Control) *SelectText {
	l := &SelectText{}
	l.initComponent(l, env, owner)
	l.LineHeight, l.RowHeight = 0x14, 0x14
	l.FocusPen, l.Highlight = 0xf5bf89, 0xf5bf89
	l.Embossed, l.FocusBox = true, true
	l.Name = "TSe_SelectText"
	l.Group, l.DownSound = 0xff, 0xff
	l.Selected, l.HoverRow, l.HighImg = -1, -1, -1
	l.ShowHover, l.ShowIcons = true, true
	l.HoverAlpha, l.SelAlpha = 0xff, 0xff
	return l
}

// SetBounds is slot +0x70 (FUN_0046f4ac): left, top, height, width; the
// number of rows shown follows from the height.
func (l *SelectText) SetBounds(left, top, height, width int) {
	l.Left, l.Top, l.Width, l.Height = left, top, width, height
	step := l.RowHeight + l.Spacing
	l.RowsShown = height / step
	if l.RowHeight <= l.Height-step*l.RowsShown {
		l.RowsShown++
	}
}

func (l *SelectText) syncScroll() {
	if l.Scroll != nil {
		l.Scroll.SetCount(len(l.Items))
	}
}

// SetCount is FUN_00474178: a new item total for the scroll button.
func (s *ScrollButton) SetCount(total int) {
	s.Total = max(total, s.Visible_)
	s.SetVertical(s.Vertical)
	if s.Pos+s.Visible_ > s.Total {
		s.SetPos(s.Total - s.Visible_)
	}
}

// Add is slot +0x74 (FUN_0046eaf4): text, icon name and data.
func (l *SelectText) Add(text []byte, icon, data string) {
	if len(text) == 0 {
		return
	}
	l.addItem(text, l.Color, icon, data)
}

// AddColored is slot +0x78 (FUN_00472c0c).
func (l *SelectText) AddColored(text []byte, color uint16, data string) {
	if len(text) == 0 {
		return
	}
	l.addItem(text, color, "", data)
}

func (l *SelectText) addItem(text []byte, color uint16, icon, data string) {
	l.Items = append(l.Items, text)
	l.Colors = append(l.Colors, color)
	l.syncScroll()
	if icon == "" {
		l.Icons = append(l.Icons, "-1")
	} else {
		l.Icons = append(l.Icons, strconv.Itoa(l.Env.Pics.Find(icon)))
	}
	l.Data = append(l.Data, data)
}

// Delete is slot +0x7c (FUN_0046ee64).
func (l *SelectText) Delete(i int) {
	if i < 0 || i >= len(l.Items) {
		return
	}
	l.Items = append(l.Items[:i], l.Items[i+1:]...)
	l.Colors = append(l.Colors[:i], l.Colors[i+1:]...)
	l.syncScroll()
	l.Icons = append(l.Icons[:i], l.Icons[i+1:]...)
	l.Data = append(l.Data[:i], l.Data[i+1:]...)
}

// Insert is slot +0x80 (FUN_0046f2c4): text at index with its data and
// icon name. The ink is the list's colour.
func (l *SelectText) Insert(text []byte, index int, data, icon string) {
	l.Items = insertAt(l.Items, index, text)
	l.Colors = insertAt(l.Colors, index, l.Color)
	l.syncScroll()
	ic := "-1"
	if icon != "" {
		ic = strconv.Itoa(l.Env.Pics.Find(icon))
	}
	l.Icons = insertAt(l.Icons, index, ic)
	l.Data = insertAt(l.Data, index, data)
}

func insertAt[T any](s []T, i int, v T) []T {
	var zero T
	s = append(s, zero)
	copy(s[i+1:], s[i:])
	s[i] = v
	return s
}

// IndexOf is FUN_0046f1c0: the first item equal to text, or -1.
func (l *SelectText) IndexOf(text []byte) int {
	for i, it := range l.Items {
		if string(it) == string(text) {
			return i
		}
	}
	return -1
}

// ResetTop is FUN_00472d98.
func (l *SelectText) ResetTop() {
	l.TopIndex = 0
	if l.Scroll != nil {
		l.Scroll.SetPos(0)
	}
}

// Clear is slot +0x84 (FUN_0046ec28).
func (l *SelectText) Clear() {
	l.Items, l.Colors, l.Icons, l.Data = nil, nil, nil, nil
	l.Selected = -1
	l.SetTop(0)
	l.syncScroll()
}

// Count is FUN_0046f2b8.
func (l *SelectText) Count() int { return len(l.Items) }

// SetTop is FUN_0046f508.
func (l *SelectText) SetTop(i int) {
	if i < 0 {
		i = 0
	}
	if i <= len(l.Items)-1 {
		l.TopIndex = i
		if l.Scroll != nil {
			l.Scroll.SetPos(i)
		}
		return
	}
	if len(l.Items) == 0 {
		l.TopIndex = 0
		if l.Scroll != nil {
			l.Scroll.SetPos(0)
		}
	}
}

// AttachScroll is FUN_00472a4c.
func (l *SelectText) AttachScroll(s *ScrollButton) {
	l.Scroll = s
	if s != nil {
		s.SetRange(len(l.Items), l.RowsShown)
		s.SetPos(l.TopIndex)
	}
}

// IndexAt is FUN_0046f264: the item under the pointer, or -1.
func (l *SelectText) IndexAt() int {
	if l.HoverRow == -1 {
		return -1
	}
	return min(l.TopIndex+l.HoverRow, len(l.Items)-1)
}

// ScrollUp is slot +0x88 (FUN_0046f474).
func (l *SelectText) ScrollUp() {
	if !l.Blocked() && l.TopIndex-1 >= 0 {
		l.TopIndex--
		if l.Scroll != nil {
			l.Scroll.SetPos(l.TopIndex)
		}
	}
}

// ScrollDown is slot +0x8c (FUN_0046f428).
func (l *SelectText) ScrollDown() {
	if !l.Blocked() && len(l.Items)-1 >= l.TopIndex+l.RowsShown {
		l.TopIndex++
		if l.Scroll != nil {
			l.Scroll.SetPos(l.TopIndex)
		}
	}
}

// PageUp and PageDown are slots +0x90 and +0x94 (FUN_004731b4,
// FUN_0047314c), used only without a scroll button.
func (l *SelectText) PageUp() {
	if l.Scroll != nil {
		return
	}
	if len(l.Items) < l.RowsShown {
		l.SetTop(0)
		return
	}
	l.SetTop(l.TopIndex - l.RowsShown)
	if l.TopIndex < 0 {
		l.SetTop(0)
	}
}

func (l *SelectText) PageDown() {
	if l.Scroll != nil {
		return
	}
	if len(l.Items) < l.RowsShown {
		l.SetTop(0)
		return
	}
	l.SetTop(l.TopIndex + l.RowsShown)
	if len(l.Items) < l.TopIndex+l.RowsShown {
		l.SetTop(len(l.Items) - l.RowsShown)
	}
}

// LeftUp is slot +0x30 (FUN_0046f090). The original runs the inherited
// click before and after selecting.
func (l *SelectText) LeftUp(shift byte, x, y int) {
	l.Component.LeftUp(shift, x, y)
	if l.Blocked() {
		return
	}
	if i := l.IndexAt(); i >= 0 {
		if i == l.Selected && l.Untoggle {
			l.Selected = -1
		} else {
			l.Selected = i
		}
		if l.OnSelect != nil {
			l.OnSelect(i)
		}
	}
	l.Component.LeftUp(shift, x, y)
}

// DblClick is slot +0x2c (FUN_0046ef5c).
func (l *SelectText) DblClick() {
	if !l.Blocked() && l.OnDblClick != nil {
		if i := l.IndexAt(); i >= 0 {
			l.OnDblClick(i)
		}
	}
}

// RightUp is slot +0x3c (FUN_0046f150).
func (l *SelectText) RightUp(shift byte, x, y int) {
	l.Component.RightUp(shift, x, y)
	if !l.Blocked() && l.OnRightUp != nil {
		if i := l.IndexAt(); i >= 0 && i <= len(l.Items)-1 {
			l.OnRightUp(i)
		}
	}
}

// MouseMove is slot +0x48 (FUN_00472b0c).
func (l *SelectText) MouseMove(byte, int, int) {
	if !l.Blocked() && l.OnMove != nil {
		if i := l.IndexAt(); i >= 0 {
			l.OnMove(i)
		}
	}
}

// Wheel slots +0x54 and +0x58 (FUN_0046f130, FUN_0046f140).
func (l *SelectText) WheelDown() { l.ScrollDown() }
func (l *SelectText) WheelUp()   { l.ScrollUp() }

// Virtual-key codes the list handles.
const (
	VKReturn = 0x0d
	VKPrior  = 0x21
	VKNext   = 0x22
	VKUp     = 0x26
	VKDown   = 0x28
)

// KeyDown is slot +0x60 (FUN_0046ef98).
func (l *SelectText) KeyDown(key uint16, _ byte) {
	if l.Blocked() {
		return
	}
	switch key {
	case VKNext:
		l.ScrollDown()
	case VKPrior:
		l.ScrollUp()
	case VKReturn:
		if l.OnSelect != nil {
			if i := l.IndexAt(); i >= 0 {
				l.OnSelect(i)
			}
		}
	case VKUp:
		if l.HoverRow-1 < 0 {
			if l.TopIndex-1 >= 0 {
				l.TopIndex--
			}
		} else {
			l.HoverRow--
		}
	case VKDown:
		if l.HoverRow+1 > l.RowsShown-1 {
			if l.TopIndex+l.RowsShown <= len(l.Items)-1 {
				l.TopIndex++
			}
		} else {
			l.HoverRow++
		}
	}
}

// Update is slot +0x18 (FUN_0046fb58): drawn first, then the hover row is
// taken from the pointer and the top row from the scroll button.
func (l *SelectText) Update(in *Input) {
	l.Component.Update(in)
	if !l.Visible {
		return
	}
	step := l.RowHeight + l.Spacing
	if !l.self.HitTest(in.X, in.Y) || in.Hovered != l.self {
		if !l.StickHover {
			l.HoverRow = -1
		}
	} else {
		dy := in.Y - l.Abs().Y
		l.HoverRow = dy / step
		l.HoverRow5 = (dy - 5) / step
		if (l.HoverRow+1)*step-dy <= l.Spacing {
			l.HoverRow, l.HoverRow5 = -1, -1
		}
		if l.HoverRow >= 0 && l.RowsShown <= l.HoverRow-l.TopIndex {
			l.HoverRow = -1
		}
		if l.HoverRow >= 0 && len(l.Items)-1 < l.HoverRow+l.TopIndex {
			if !l.StickHover {
				l.HoverRow = -1
			} else {
				l.HoverRow = len(l.Items) - 1 - l.TopIndex
			}
		}
	}
	if l.Scroll != nil && l.Scroll.Pos != l.TopIndex {
		l.TopIndex = l.Scroll.Pos
		if l.OnScroll != nil {
			l.OnScroll()
		}
	}
}

// fill is FUN_000737cc: a TColor rectangle on the surface.
func (l *SelectText) fill(r image.Rectangle, c uint32) {
	l.Env.Screen.Fill(r, surface.RGB565(uint8(c), uint8(c>>8), uint8(c>>16)))
}

// Paint is slot +0x10 (FUN_0046f56c): focus box and hover highlight while
// the list is under the pointer, the selection highlight, then each shown row's icon and text. Alpha-blended
// highlights (+0x15c, +0x15e) and highlight images (+0x164) are not used
// by the login screens and are drawn as plain fills.
func (l *SelectText) Paint() {
	o := l.ParentOrigin()
	x, y := o.X+l.Left, o.Y+l.Top
	step := l.RowHeight + l.Spacing
	hovered := l.Env.UI != nil && l.Env.UI.Input.Hovered == l.self
	if !l.Blocked() && (hovered || l.FocusAlways) && l.FocusBox {
		r := image.Rect(x-1, y-3, x+2+l.Width, y+l.Height)
		c := surface.RGB565(uint8(l.FocusPen), uint8(l.FocusPen>>8), uint8(l.FocusPen>>16))
		scr := l.Env.Screen
		scr.Fill(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+2), c)
		scr.Fill(image.Rect(r.Min.X, r.Max.Y-2, r.Max.X, r.Max.Y), c)
		scr.Fill(image.Rect(r.Min.X, r.Min.Y, r.Min.X+2, r.Max.Y), c)
		scr.Fill(image.Rect(r.Max.X-2, r.Min.Y, r.Max.X, r.Max.Y), c)
	}
	if l.ShowHover && (l.StickHover || hovered) && l.HoverRow < l.RowsShown && l.HoverRow >= 0 &&
		l.TopIndex+l.HoverRow < len(l.Items) && len(l.Items[l.TopIndex+l.HoverRow]) > 0 {
		r := image.Rect(x, y-2+step*l.HoverRow, x+l.Width, y-2+step*l.HoverRow+l.RowHeight)
		l.fill(r, l.Highlight)
	}
	if l.ShowSel && l.Selected >= 0 && l.Selected < len(l.Items) &&
		l.TopIndex <= l.Selected && l.Selected < l.TopIndex+l.RowsShown {
		row := l.Selected - l.TopIndex
		r := image.Rect(x, y-2+step*row, x+l.Width, y-2+step*row+l.RowHeight)
		l.fill(r, l.Highlight)
	}
	style := byte(0)
	if l.Embossed {
		style = 2
	}
	for row := 0; row < l.RowsShown; row++ {
		i := l.TopIndex + row
		if i >= len(l.Items) || len(l.Items[i]) == 0 {
			continue
		}
		if l.ShowIcons && len(l.Icons) > 0 {
			if icon, err := strconv.Atoi(l.Icons[i]); err == nil && icon != -1 {
				l.Env.Pics.Draw(l.Env.Screen, icon, x+2, y+step*row-2+l.IconY, true)
			}
		}
		l.Env.Text.Draw(x+2+l.TextX, y+step*row+l.TextY, 0, false, true, l.Env.Screen,
			l.Items[i], l.LineHeight, l.Width, l.Color2, l.Colors[i], style)
	}
}
