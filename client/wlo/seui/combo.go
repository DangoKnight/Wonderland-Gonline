package seui

import (
	"image"

	"wonderland-gonline/client/wlo/surface"
)

// ComboBox is TCJ_ComboBox (constructor FUN_0047af90): an editor with a
// drop button that opens a list of earlier entries below it.
type ComboBox struct {
	Editor
	MaxItems  byte            // +0x248
	List      *SelectText     // +0x24c
	Button    *FixedButton    // +0x250
	Scroll    *ScrollButton   // +0x254
	BackImg   int             // +0x258 Icon_AccountBack
	ButtonOff image.Point     // +0x218, +0x21c
	ScrollOff image.Point     // +0x220, +0x224
	ListOff   image.Point     // +0x228, +0x22c
	ScrollW   int             // +0x230
	ScrollH   int             // +0x234
	Back      image.Rectangle // +0x238 the drop-down's area

	// OnRemove is slot +0x94, the list's right-click (TAccountList deletes
	// the entry; the combo box itself ignores it, FUN_0047b8d4).
	OnRemove func(i int)
}

func NewComboBox(env *Env, owner Control) *ComboBox {
	c := &ComboBox{}
	c.initCombo(c, env, owner)
	return c
}

func (c *ComboBox) initCombo(self Control, env *Env, owner Control) {
	c.initEditor(self, env, owner)
	c.MaxItems = 5
	c.ListOff = image.Pt(0, 4)
	c.ScrollW, c.ScrollH = 0x11, 0x3e
	c.Scroll = NewScrollButton(env, self)
	c.Scroll.SetVisible(false)
	c.Button = NewFixedButton(env, self)
	c.List = NewSelectText(env, self)
	c.List.SetVisible(false)
	// The original loads the picture from a "default\" directory when the
	// skins lack it; that path does not exist in the client tree.
	c.BackImg = env.Pics.Find("Icon_AccountBack")
}

// Setup is FUN_0047b1dc in Ghidra's argument order: button picture, button
// x offset, scroll y offset, scroll x offset, rail and thumb pictures, and
// button y offset. Positions subtract the control's screen position at
// the time of the call, as the original does while its form is at 0,0.
func (c *ComboBox) Setup(button string, buttonX, scrollY, scrollX int, rail, thumb string, buttonY int) {
	c.ButtonOff = image.Pt(buttonX, buttonY)
	c.setupButton(button)
	c.ScrollOff = image.Pt(scrollX, scrollY)
	c.setupScroll(thumb, rail)
	c.Back = bounds(c.Left, c.Top-c.Height, c.Width, c.Scroll.Height)
	c.setupList()
}

// bounds is FUN_00020d08: a rectangle from position and size.
func bounds(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

// setupButton is FUN_0047b32c: a three-row button at the right edge.
func (c *ComboBox) setupButton(name string) {
	if name == "" {
		name = "Btn_ArrowDn_2"
	}
	w, h := c.Env.Pics.Size(c.Env.Pics.Find(name))
	h /= 3
	a := c.Abs()
	top := c.Top - a.Y + c.ButtonOff.Y
	c.Button.Init(name, c.Left+c.Width-a.X+c.ButtonOff.X, h, w, 0, 0, true, h, w, top)
	c.Button.OnClick = c.Toggle
}

// setupScroll is FUN_0047b438.
func (c *ComboBox) setupScroll(thumb, rail string) {
	if thumb == "" {
		thumb = "bar_H4"
	}
	w, h := c.Env.Pics.Size(c.Env.Pics.Find(thumb))
	c.Scroll.SetThumb(thumb, 0, h, w/3, 0)
	if rail == "" {
		rail = "Rail_H5"
	}
	w, h = c.Env.Pics.Size(c.Env.Pics.Find(rail))
	a := c.Abs()
	top := c.Top + c.Height - a.Y + c.ScrollOff.Y
	c.Scroll.Init(rail, c.Left+c.Width-a.X+c.ScrollOff.X, h, w, 0, 0, true, c.ScrollH, c.ScrollW, top)
	c.Scroll.Inset = 3
	c.Scroll.SetCaps(7, 0x10)
	c.Scroll.TabStop = false
}

// setupList is FUN_0047b5e0.
func (c *ComboBox) setupList() {
	l := c.List
	l.Spacing = 0
	a := c.Abs()
	l.SetBounds(c.Left+1-a.X, c.Top+c.Height-a.Y+c.ListOff.Y, c.Scroll.Height, c.Width-2)
	l.Name = "TextList"
	l.ShowIcons = false
	l.SetColor(0x841)
	l.Highlight = 0xffff
	l.Embossed, l.FocusBox, l.ShowHover = false, false, false
	l.AttachScroll(c.Scroll)
	l.OnSelect = c.choose
	l.OnRightUp = func(i int) {
		if c.OnRemove != nil {
			c.OnRemove(i)
		}
	}
}

// Toggle is FUN_0047b814, the drop button's click.
func (c *ComboBox) Toggle() {
	if c.List.Count() <= 0 {
		return
	}
	c.List.SetVisible(!c.List.Visible)
	c.List.ShowHover = c.List.Visible
	c.List.ResetTop()
	c.Scroll.SetVisible(c.List.Visible)
	c.resize()
}

// resize is FUN_0047b2b4: the drop-down shows up to three rows without a
// scroll bar.
func (c *ComboBox) resize() {
	if !c.List.Visible {
		return
	}
	h := c.List.Count()*0x14 + 2
	if c.List.Count() < 4 {
		c.Scroll.SetVisible(false)
	} else {
		h = c.ScrollH
	}
	c.Back = bounds(c.Left, c.Top-c.Height, c.Width, h)
}

// choose is FUN_0047b868, the list's selection.
func (c *ComboBox) choose(i int) {
	c.SetText(c.List.Items[i])
	c.Toggle()
	if c.Env.UI != nil {
		c.Env.UI.Focus(c.self)
	}
}

// AddItem is FUN_0047b8d8: the entry moves to the top; a new one drops
// the last when the list is full. For an existing entry the original
// removes only the text, leaving its colour, icon and data behind.
func (c *ComboBox) AddItem(text []byte) {
	l := c.List
	if i := l.IndexOf(text); i == -1 {
		if l.Count() >= int(c.MaxItems) {
			l.Delete(l.Count() - 1)
		}
		l.Insert(text, 0, "0", "")
	} else {
		existing := l.Items[i]
		l.Items = append(l.Items[:i:i], l.Items[i+1:]...)
		l.Insert(existing, 0, "0", "")
	}
}

// Paint is slot +0x10 (FUN_0047b6dc): the editor, then the open drop-down
// (FUN_0047b6f0), which closes when focus leaves it.
func (c *ComboBox) Paint() {
	c.Editor.Paint()
	if !c.List.Visible {
		return
	}
	if !c.Button.Focused && !c.List.Focused && !c.Scroll.Focused {
		c.Toggle()
	}
	if c.BackImg != -1 {
		c.Env.Pics.Draw(c.Env.Screen, c.BackImg, c.Left, c.Top-c.Height, true)
	} else {
		// The canvas path: an opaque black fill and a navy outline.
		c.Env.Screen.FillAlpha(c.Back, 0, opaque)
		c.Env.Screen.Frame(c.Back, surface.TColor(navy))
	}
	c.List.Paint()
}

// Drop-down outline (TColor) and fill opacity for the canvas path.
const (
	navy   = 0x800000
	opaque = 0xff
)

// AccountList is TAccountList (constructor FUN_00402afc): a combo box of
// remembered accounts kept in user\AccountList.dat.
type AccountList struct {
	ComboBox
	// Save writes the entries, or removes the file when there are none
	// (FUN_00402c30, FUN_00019870).
	Save func(items [][]byte)
}

func NewAccountList(env *Env, owner Control) *AccountList {
	a := &AccountList{}
	a.initCombo(a, env, owner)
	a.OnRemove = a.remove
	return a
}

// remove is slot +0x94 (FUN_00402b5c).
func (a *AccountList) remove(i int) {
	a.List.Delete(i)
	if a.Save != nil {
		a.Save(a.Entries())
	}
	a.resize()
	if a.List.Count() < 1 {
		a.List.SetVisible(false)
		a.List.ShowHover = false
		a.Scroll.SetVisible(false)
	}
}

// Entries are the first MaxItems texts, as FUN_00402c30 saves them.
func (a *AccountList) Entries() [][]byte {
	n := min(a.List.Count(), int(a.MaxItems))
	return a.List.Items[:n]
}

// LoadEntries is FUN_00402d48 given the file's lines.
func (a *AccountList) LoadEntries(lines [][]byte) {
	a.List.Clear()
	for i, s := range lines {
		if i >= int(a.MaxItems) {
			break
		}
		a.List.Add(s, "", "")
	}
}
