package seui

import (
	"image"
	"time"
)

// Memo is TSe_Memo (constructor FUN_0046c178, bounds FUN_0046d4c4, paint
// FUN_0046d52c): multi-line text wrapped at Columns characters, Rows lines
// of CharH + 2 pixels, drawn from (TextX, TextY) in the memo with the
// blinking inputCursor2 caret. Chat emoticon codes may be drawn by
// DrawText. This port types and deletes at the end of the text only, and
// keeps no scroll bar (+0x148): text that does not fit is not shown; the
// original's caret movement and selection keys are not ported.
type Memo struct {
	Component
	Text      []byte // +0xdc
	MaxLen    int    // +0xe4
	CharW     int    // +0x140
	CharH     int    // +0x144
	Columns   int    // FUN_0046de98's width in characters
	Rows      int    // +0xf4
	TextX     int    // +0xfc
	TextY     int    // +0x100
	TextStyle byte   // +0x169
	ReadOnly  bool
	CursorImg int // +0x4c inputCursor2
	blinkOn   bool
	blinkAt   time.Time
	Now       func() time.Time
	// DrawText draws one line at screen coordinates when set.
	DrawText func(x, y int, line []byte)
	OnChange func()
}

const (
	memoCaretW = 3
	memoBlink  = 300 * time.Millisecond
)

// NewMemo is FUN_0046c178: 8 × 15 characters, text colour $0841, style 2.
func NewMemo(env *Env, owner Control) *Memo {
	m := &Memo{CharW: 8, CharH: 0xf, TextStyle: 2, Now: time.Now}
	m.initComponent(m, env, owner)
	m.Name = "TSe_Memo"
	m.Color = 0x0841
	m.CursorImg = env.Pics.Find("inputCursor2")
	m.Image = -1
	return m
}

// SetBounds is FUN_0046d4c4: position and size; the columns leave one
// character's margin and the rows are CharH + 2 high.
func (m *Memo) SetBounds(left, top, width, height int) {
	m.Left, m.Top, m.Width, m.Height = left, top, width, height
	m.Columns = (width - m.CharW) / m.CharW
	m.Rows = height / (m.CharH + 2)
}

// SetText replaces the text.
func (m *Memo) SetText(s []byte) {
	m.Text = append([]byte(nil), s...)
	if m.MaxLen > 0 && len(m.Text) > m.MaxLen {
		m.Text = m.Text[:m.MaxLen]
	}
}

// Lines wraps the text at Columns, never splitting a double-byte
// character, and at line breaks.
func (m *Memo) Lines() [][]byte {
	var out [][]byte
	var line []byte
	cols := max(m.Columns, 2)
	for i := 0; i < len(m.Text); {
		c := m.Text[i]
		if c == '\r' || c == '\n' {
			out = append(out, line)
			line = nil
			i++
			continue
		}
		n := 1
		if c > 0x7f && i+1 < len(m.Text) {
			n = 2
		}
		if len(line)+n > cols {
			out = append(out, line)
			line = nil
		}
		line = append(line, m.Text[i:i+n]...)
		i += n
	}
	return append(out, line)
}

// Char types one byte (FUN_0046c9d0): Backspace deletes the last
// character, Enter starts a line; MaxLen bounds the text.
func (m *Memo) Char(c byte) {
	if m.ReadOnly {
		return
	}
	switch c {
	case VKBack:
		if n := len(m.Text); n > 0 {
			cut := 1
			if n >= 2 && m.Text[n-2] > 0x7f && !m.lastIsSingle() {
				cut = 2
			}
			m.Text = m.Text[:n-cut]
		}
	case VKReturn:
		if m.MaxLen > 0 && len(m.Text) >= m.MaxLen {
			return
		}
		m.Text = append(m.Text, '\r')
	case VKTab, VKEscape:
		return
	default:
		if m.MaxLen > 0 && len(m.Text) >= m.MaxLen {
			return
		}
		m.Text = append(m.Text, c)
	}
	if m.OnChange != nil {
		m.OnChange()
	}
}

// lastIsSingle reports whether the last byte is a character of its own
// (counting lead bytes from the start).
func (m *Memo) lastIsSingle() bool {
	i := 0
	for i < len(m.Text)-1 {
		if m.Text[i] > 0x7f {
			i += 2
		} else {
			i++
		}
	}
	return i == len(m.Text)-1
}

// KeyDown leaves the navigation keys to the original (not ported).
func (m *Memo) KeyDown(key uint16, shift byte) {}

// Paint is FUN_0046d52c: the visible lines, then the caret after the text
// while focused.
func (m *Memo) Paint() {
	m.Component.Paint()
	o := m.Abs()
	lines := m.Lines()
	first := max(len(lines)-m.Rows, 0)
	for i := 0; i < m.Rows && first+i < len(lines); i++ {
		x, y := o.X+m.TextX, o.Y+m.TextY+(m.CharH+2)*i
		if m.DrawText != nil {
			m.DrawText(x, y, lines[first+i])
		} else if len(lines[first+i]) > 0 {
			m.Env.Text.Draw(x, y, 0, false, true, m.Env.Screen, lines[first+i], m.CharH, m.Width, m.Color2, m.Color, m.TextStyle)
		}
	}
	if !m.Focused || m.ReadOnly {
		return
	}
	if now := m.Now(); now.Sub(m.blinkAt) > memoBlink {
		m.blinkAt, m.blinkOn = now, !m.blinkOn
	}
	if !m.blinkOn || m.CursorImg < 0 {
		return
	}
	row := min(len(lines)-1-first, m.Rows-1)
	col := len(lines[len(lines)-1])
	x := o.X + m.TextX + col*m.CharW - 1
	y := o.Y + m.TextY + row*(m.CharH+2)
	m.Env.Pics.DrawRect(m.Env.Screen, m.CursorImg, x, y, image.Rect(0, 0, memoCaretW, m.CharH), true)
}
