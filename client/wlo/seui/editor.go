package seui

import (
	"image"
	"time"
)

// Editor is TSe_Editor (constructor FUN_00468c4c): a one-line text field
// drawn over a panel, with an optional label and a blinking caret.
//
// Positions follow the original: Caret is the number of bytes before the
// caret and the helpers take 1-based byte positions as Delphi strings do.
// Chat-specific tokens use optional hooks; ordinary fields retain Big5 editing.
type Editor struct {
	Panel
	Align      byte      // +0x121: 0 left, 2 right
	FontSize   byte      // +0x122
	CharW      int       // +0x1fc
	CharH      int       // +0x200
	Label      []byte    // +0x48
	LabelX     int       // +0x40
	LabelY     int       // +0x44
	Text       []byte    // +0x1a0, length +0x1a8
	MaxLen     int       // +0x1b4
	Limit      bool      // +0x1b8
	TextX      int       // +0x1ac
	TextY      int       // +0x1b0
	Columns    int       // +0x1bc characters that fit
	pending    bool      // +0x1c0 a double-byte lead is waiting
	unit       []byte    // +0x1c4 the character being typed
	Filter     byte      // +0x1c8: 1 digits, 2 letters and digits, 3 single bytes alphanumeric
	Caret      int       // +0x208
	CaretCol   int       // +0x20c column of the caret in the visible text
	Insert     bool      // +0x210 cleared by the Insert key for overwrite
	ReadOnly   bool      // +0x211
	Password   bool      // +0x1f8
	suppress   bool      // +0x204 drops the character after Ctrl+V
	AllowPaste bool      // +0x205
	TextStyle  byte      // +0x212
	CursorImg  int       // +0x1cc "inputCursor"
	blinkOn    bool      // +0x120
	blinkAt    time.Time // +0x118
	Now        func() time.Time

	// TokenWidth recognizes extra two-byte units, such as chat emoticons.
	TokenWidth func([]byte) int
	// DrawText optionally renders visible non-password text at screen coordinates.
	DrawText func(x, y int, text []byte)

	OnBlur   func() // validation when the editor loses keyboard focus
	OnEnter  func() // +0x1d0
	OnTab    func() // +0x1d8
	OnChange func() // +0x1e0
	OnUp     func() // +0x1e8 Up and Page Up
	OnDown   func() // +0x1f0 Down and Page Down

	// Clipboard reads and ClearClipboard empties the system clipboard
	// (TClipboard at DAT_007c1cbc) for Ctrl+V.
	Clipboard      func() []byte
	ClearClipboard func()
}

// Blink is the caret period (_DAT_0046a8ec, 300 ms).
const Blink = 300 * time.Millisecond

func NewEditor(env *Env, owner Control) *Editor {
	e := &Editor{}
	e.initEditor(e, env, owner)
	return e
}

func (e *Editor) initEditor(self Control, env *Env, owner Control) {
	e.initPanel(self, env, owner)
	e.Name = "editor"
	e.Align = 0
	e.setFontSize(0)
	e.Color, e.Color2 = 0xffff, 0
	e.Image = -1
	e.CursorImg = env.Pics.Find("inputCursor")
	e.SetMargins(4, 4, 4, 4)
	e.TabStop, e.Hover = false, false
	e.ReadOnly, e.Insert = false, true
	e.Group, e.DownSound = 0xff, 0xff
	e.AllowPaste = false
	e.Now = time.Now
}

// setFontSize is FUN_00468d94.
func (e *Editor) setFontSize(size byte) {
	e.FontSize = size
	switch size {
	case 0:
		e.CharW, e.CharH = 8, 0xf
	case 1:
		e.CharW, e.CharH = 10, 0x13
	case 2:
		e.CharW, e.CharH = 0xc, 0x18
	}
}

// Init is slot +0x08 (FUN_0046a8f0): TSe_GrBasic's, then the text layout.
func (e *Editor) Init(name string, left, clipH, clipW, clipY, clipX int, useClip bool, height, width, top int) {
	e.GrBasic.Init(name, left, clipH, clipW, clipY, clipX, useClip, height, width, top)
	e.layout()
}

// layout is FUN_00468eb4.
func (e *Editor) layout() {
	lw := len(e.Label)*e.CharW + 4
	e.LabelX = e.Left + 2
	e.Columns = (e.Width - lw - 2) / e.CharW
	switch e.Align {
	case 0:
		e.TextX = lw + e.Left
		e.Caret, e.CaretCol = 0, 0
	case 2:
		e.TextX = e.Left + e.Width - ((e.Width-lw-2)%e.CharW + 2)
		e.Caret, e.CaretCol = len(e.Text), e.Columns
	}
	// Go's division truncates toward zero like the original's sar fix-up.
	e.TextY = (e.Height-e.CharH)/2 + e.Top
	e.LabelY = e.TextY
}

// SetLabel sets the caption drawn before the text (+0x48) and lays out.
func (e *Editor) SetLabel(s []byte) { e.Label = s; e.layout() }

// SetAlign is FUN_00468d88.
func (e *Editor) SetAlign(a byte) { e.Align = a; e.layout() }

// SetFilter is FUN_00468eac.
func (e *Editor) SetFilter(f byte) { e.Filter = f }

// SetMaxLen is FUN_00468e8c.
func (e *Editor) SetMaxLen(n int) {
	e.MaxLen = n
	if n > 0 {
		e.Limit = true
	} else if n == 0 {
		e.Limit = false
	}
}

// SetText is FUN_00468de8: the text is replaced and an End key press moves
// the caret after it.
func (e *Editor) SetText(s []byte) {
	e.Text = append([]byte(nil), s...)
	e.self.KeyDown(VKEnd, 0)
}

// Clear is FUN_00468e64.
func (e *Editor) Clear() {
	e.Text = nil
	e.layout()
}

const (
	VKBack   = 0x08
	VKTab    = 0x09
	VKEscape = 0x1b
	VKEnd    = 0x23
	VKHome   = 0x24
	VKLeft   = 0x25
	VKRight  = 0x27
	VKInsert = 0x2d
	VKDelete = 0x2e
	VKV      = 0x56
	// ShiftCtrl is the TShiftState for Ctrl alone (DAT_00469df0).
	ShiftCtrl = 4
)

// at is the 1-based byte at pos; past the end it reads the terminator.
func (e *Editor) at(pos int) byte {
	if pos < 1 || pos > len(e.Text) {
		return 0
	}
	return e.Text[pos-1]
}

func (e *Editor) tokenAt(pos int) bool {
	return e.TokenWidth != nil && pos >= 1 && pos <= len(e.Text) && e.TokenWidth(e.Text[pos-1:]) == 2
}

// walk steps over whole characters from position 1 while short of pos and
// returns where it stops.
func (e *Editor) walk(pos int) int {
	i := 1
	for i < pos {
		if e.at(i) < 0x80 && !e.tokenAt(i) {
			i++
		} else {
			i += 2
		}
	}
	return i
}

// charStart is FUN_00468fd4: whether byte pos begins a character.
func (e *Editor) charStart(pos int) bool {
	if len(e.Text) < 2 || pos <= 1 || pos > len(e.Text) {
		return true
	}
	return e.walk(pos) == pos
}

// single is FUN_00469090: whether byte pos is a single-byte character (or
// the trail of a double-byte one).
func (e *Editor) single(pos int) bool {
	if pos < 1 || pos > len(e.Text) {
		return false
	}
	if e.walk(pos) == pos+1 {
		return true
	}
	return e.at(pos) < 0x80 && !e.tokenAt(pos)
}

// sizeBefore is FUN_00468f70: the size of the character ending at byte pos.
func (e *Editor) sizeBefore(pos int) int {
	if pos == 0 || pos > len(e.Text) {
		return 0
	}
	if e.charStart(pos) {
		return 1
	}
	return 2
}

// sizeAfter is FUN_00468fa0: the size of the character after the caret
// position pos.
func (e *Editor) sizeAfter(pos int) int {
	if pos < 0 || pos >= len(e.Text) {
		return 0
	}
	if e.single(pos + 1) {
		return 1
	}
	return 2
}

// colBack is FUN_00469190: the caret column after scrolling left.
func (e *Editor) colBack() int {
	switch e.Align {
	case 0:
		if e.Caret <= 4 {
			return e.Caret
		}
		if e.at(e.Caret-1) < 0x7f && !e.tokenAt(e.Caret-1) {
			return 1
		}
		return 2
	case 2:
		if e.single(e.Caret + e.Columns - 5) {
			return 5
		}
		return 4
	}
	return 0
}

// colForward is FUN_00469244: the caret column after scrolling right by n
// bytes (0 for an insertion at the end). The original counts in 16 bits.
func (e *Editor) colForward(n int) int {
	switch e.Align {
	case 0:
		if n < 1 {
			end := uint16(len(e.Text) - e.Columns + 6)
			i := uint16(1)
			if end > 1 {
				for i < end {
					if e.at(int(i)) < 0x7f && !e.tokenAt(int(i)) {
						i++
					} else {
						i += 2
					}
				}
			}
			if end < i {
				return e.Columns - 4
			}
			return e.Columns - 3
		}
		s := e.Caret - n - e.CaretCol
		i, end := uint16(s+1), uint16(s+5)
		for i < end {
			if e.at(int(i)) < 0x81 && !e.tokenAt(int(i)) {
				i++
			} else {
				i += 2
			}
		}
		col := e.Columns - 4
		if end < i {
			col = e.Columns - 3
		}
		if !e.charStart(e.Caret - col + 1) {
			col++
		}
		return col
	case 2:
		if e.Caret < len(e.Text)-4 {
			if e.single(e.Caret + 4) {
				return e.Columns - 4
			}
			return e.Columns - 5
		}
		return e.Columns - len(e.Text) + e.Caret
	}
	return 0
}

// colEnd is FUN_004693d8.
func (e *Editor) colEnd() int {
	if e.charStart(len(e.Text) - e.Columns + 1) {
		return e.Columns
	}
	return e.Columns - 1
}

// colHome is FUN_00469408.
func (e *Editor) colHome() int {
	if e.single(e.Columns) {
		return 0
	}
	return 1
}

func (e *Editor) changed() {
	if e.OnChange != nil {
		e.OnChange()
	}
}

func isDigit(c byte) bool { return c-'0' < 10 }
func isLower(c byte) bool { return c-'a' < 26 }
func isUpper(c byte) bool { return c-'A' < 26 }
func isAlnum(c byte) bool { return isDigit(c) || isLower(c) || isUpper(c) }

// Char is FUN_0046942c: a typed character is filtered, joined to a waiting
// lead byte, and put at the caret.
func (e *Editor) Char(c byte) {
	if e.ReadOnly || e.suppress {
		return
	}
	if c == VKBack {
		e.changed()
	}
	if !e.pending && (c == VKBack || c == VKTab || c == VKReturn || c == VKEscape) {
		return
	}
	if e.Filter == 1 {
		if !isDigit(c) {
			return
		}
		// A digit typed into a numeric field showing "0" replaces it.
		if e.Name == "editor" && string(e.Text) == "0" {
			e.Text = nil
			e.Caret = 0
			if e.Align == 0 {
				e.CaretCol = 0
			}
		}
	}
	if e.Filter == 2 && !isAlnum(c) {
		return
	}
	if e.Name == "IDField" && isLower(c) {
		c -= 0x20
	}
	if e.Name == "CreateRoleName" && c == ' ' {
		return
	}
	if e.pending {
		// FUN_00486a84/FUN_000927e4 convert the pair on one Windows
		// version; Big5 input needs no conversion.
		e.unit = append(e.unit, c)
		e.pending = false
	} else {
		e.unit = []byte{c}
		if c >= 0x80 {
			e.pending = true
			return
		}
	}
	if e.Filter == 3 && len(e.unit) == 1 && !isAlnum(e.unit[0]) {
		return
	}
	if len(e.unit) == 1 && e.unit[0] == 0x16 { // DAT_004698f0, Ctrl+V
		return
	}
	n := len(e.unit)
	if !e.Insert && e.Caret != len(e.Text) {
		var width int
		if e.single(e.Caret + 1) {
			if e.Limit && len(e.Text)+n-1 > e.MaxLen {
				return
			}
			width = 1
		} else {
			width = 2
		}
		e.Text = append(e.Text[:e.Caret:e.Caret], append(append([]byte(nil), e.unit...), e.Text[e.Caret+width:]...)...)
		e.Caret += n
		switch e.Align {
		case 0:
			if e.CaretCol+n > e.Columns {
				e.CaretCol = e.colForward(0)
			} else {
				e.CaretCol += n
			}
		case 2:
			col := e.CaretCol + width
			switch {
			case col < n:
				e.CaretCol = e.colBack()
			case col > e.Columns:
				e.CaretCol = e.colForward(0)
			default:
				e.CaretCol = col
			}
		}
	} else {
		if e.Limit && len(e.Text)+n > e.MaxLen {
			return
		}
		e.Text = append(e.Text[:e.Caret:e.Caret], append(append([]byte(nil), e.unit...), e.Text[e.Caret:]...)...)
		e.Caret += n
		switch e.Align {
		case 0:
			if e.CaretCol+n > e.Columns {
				e.CaretCol = e.colForward(0)
			} else {
				e.CaretCol += n
			}
		case 2:
			if n > e.CaretCol {
				e.CaretCol = e.colBack()
			}
		}
	}
	e.changed()
}

func (e *Editor) remove(at, n int) {
	e.Text = append(e.Text[:at:at], e.Text[at+n:]...)
}

// KeyDown is slot +0x60 (FUN_004698f4).
func (e *Editor) KeyDown(key uint16, shift byte) {
	if e.ReadOnly {
		return
	}
	e.suppress = false
	if shift == ShiftCtrl && key == VKV {
		if e.AllowPaste && e.Clipboard != nil {
			s := e.Clipboard()
			if e.Password && e.ClearClipboard != nil {
				e.ClearClipboard()
			}
			for _, c := range s {
				e.Char(c)
			}
		}
		e.suppress = true
		return
	}
	switch key {
	case VKBack:
		if len(e.Text) == 0 {
			return
		}
		n := e.sizeBefore(e.Caret)
		if n == 0 {
			return
		}
		e.remove(e.Caret-n, n)
		e.Caret -= n
		switch e.Align {
		case 0:
			if e.CaretCol < 1 {
				e.CaretCol = e.colBack()
			} else {
				e.CaretCol -= n
			}
		case 2:
			if e.CaretCol < n {
				e.CaretCol = e.colBack()
			}
		}
	case VKReturn:
		if e.OnEnter != nil {
			e.OnEnter()
		}
	case VKEscape:
		e.Clear()
	case VKPrior, VKUp:
		if e.OnUp != nil {
			e.OnUp()
		}
	case VKNext, VKDown:
		if e.OnDown != nil {
			e.OnDown()
		}
	case VKEnd:
		e.Caret = len(e.Text)
		switch e.Align {
		case 0:
			if len(e.Text) < e.Columns {
				e.CaretCol = len(e.Text)
			} else {
				e.CaretCol = e.colEnd()
			}
		case 2:
			e.CaretCol = e.Columns
		}
	case VKHome:
		e.Caret = 0
		switch e.Align {
		case 0:
			e.CaretCol = 0
		case 2:
			if e.Columns < len(e.Text) {
				e.CaretCol = e.colHome()
			} else {
				e.CaretCol = e.Columns - len(e.Text)
			}
		}
	case VKLeft:
		n := e.sizeBefore(e.Caret)
		if n == 0 {
			return
		}
		e.Caret -= n
		switch e.Align {
		case 0:
			if e.CaretCol < 1 {
				e.CaretCol = e.colBack()
			} else {
				e.CaretCol -= n
			}
		case 2:
			if e.CaretCol < n {
				e.CaretCol = e.colBack()
			} else {
				e.CaretCol -= n
			}
		}
	case VKRight:
		n := e.sizeAfter(e.Caret)
		if n == 0 {
			return
		}
		e.Caret += n
		switch e.Align {
		case 0:
			if e.CaretCol+n > e.Columns {
				e.CaretCol = e.colForward(n)
			} else {
				e.CaretCol += n
			}
		case 2:
			if e.CaretCol < e.Columns {
				e.CaretCol += n
			} else {
				e.CaretCol = e.colForward(0)
			}
		}
	case VKInsert:
		e.Insert = !e.Insert
	case VKDelete:
		if len(e.Text) == 0 {
			return
		}
		n := e.sizeAfter(e.Caret)
		if n == 0 {
			return
		}
		e.remove(e.Caret, n)
		switch e.Align {
		case 0:
			if n+e.CaretCol > e.Columns {
				e.CaretCol = e.colForward(0)
			}
		case 2:
			if e.CaretCol < e.Columns {
				e.CaretCol += n
			} else {
				e.CaretCol = e.colForward(0)
			}
		}
	}
}

// KeyUp is slot +0x64 (FUN_0046aa9c): Tab while the form is shown.
func (e *Editor) KeyUp(key uint16, shift byte) {
	if e.Root().Base().Visible && e.Visible && key == VKTab && e.OnTab != nil {
		e.OnTab()
	}
}

// LeftDown is slot +0x38 (FUN_0046a96c): the caret restarts visible.
func (e *Editor) LeftDown(shift byte, x, y int) {
	e.Component.LeftDown(shift, x, y)
	e.blinkAt = e.Now()
	e.blinkOn = true
}

// visible is the window of the text that is drawn (FUN_00469df4).
func (e *Editor) visible() []byte {
	var from, to int
	switch e.Align {
	case 0:
		from = e.Caret - e.CaretCol + 1
		if e.single(e.Columns + from - 1) {
			to = e.Columns + from - 1
		} else {
			to = e.Columns + from - 2
		}
	case 2:
		to = e.Caret + e.Columns - e.CaretCol
		if e.charStart(to + 1 - e.Columns) {
			from = to + 1 - e.Columns
		} else {
			from = to + 2 - e.Columns
		}
	}
	return copyStr(e.Text, from, to-from+1)
}

// copyStr is Delphi's Copy(s, index, count).
func copyStr(s []byte, index, count int) []byte {
	if index < 1 {
		index = 1
	}
	if index > len(s) || count <= 0 {
		return nil
	}
	return s[index-1 : min(index-1+count, len(s))]
}

// Paint is slot +0x10 (FUN_00469df4): the panel, the label, the visible
// text (or, for a password, one asterisk per byte and no label), then the
// caret while focused.
func (e *Editor) Paint() {
	o := e.ParentOrigin()
	draw := func(x, y int, s []byte, paper, ink uint16) {
		e.Env.Text.Draw(x, y, 0, false, true, e.Env.Screen, s, e.CharH, e.Width, paper, ink, e.TextStyle)
	}
	textAt := func(s []byte) int {
		if e.Align == 2 {
			return e.TextX - len(s)*e.CharW
		}
		return e.TextX
	}
	if !e.Password {
		shown := e.visible()
		e.Panel.Paint()
		if len(e.Label) > 0 {
			draw(e.LabelX+o.X, e.LabelY+o.Y, e.Label, 0, e.Color3)
		}
		if len(shown) > 0 {
			if e.DrawText != nil {
				e.DrawText(o.X+textAt(shown), e.TextY+o.Y, shown)
			} else {
				draw(o.X+textAt(shown), e.TextY+o.Y, shown, e.Color2, e.Color)
			}
		}
	} else {
		stars := make([]byte, len(e.Text))
		for i := range stars {
			stars[i] = '*'
		}
		e.Panel.Paint()
		if len(stars) > 0 {
			draw(o.X+textAt(stars), e.TextY+o.Y, stars, e.Color2, e.Color)
		}
	}
	if !e.Focused {
		return
	}
	x := e.CaretCol*e.CharW + e.TextX
	if e.Align == 2 {
		x = e.TextX - (e.Columns-e.CaretCol)*e.CharW
	}
	if now := e.Now(); now.Sub(e.blinkAt) > Blink {
		e.blinkAt = now
		e.blinkOn = !e.blinkOn
	}
	if e.blinkOn {
		e.Env.Pics.DrawRect(e.Env.Screen, e.CursorImg, o.X+x-1, e.TextY+o.Y, image.Rect(0, 0, 7, e.CharH), true)
	}
}

// Update is slot +0x18 (FUN_0046a9b0): focus is only tracked while the
// editor accepts input.
func (e *Editor) Update(in *Input) {
	if !e.Visible {
		return
	}
	if e.self.HitTest(in.X, in.Y) {
		in.Hovered = e.self
	}
	if !e.Blocked() && !e.ReadOnly {
		wasFocused := e.Focused
		e.Focused = in.Focused == e.self
		if wasFocused && !e.Focused && e.OnBlur != nil {
			e.OnBlur()
		}
	}
	e.self.Paint()
	e.updateChildren(in)
}
