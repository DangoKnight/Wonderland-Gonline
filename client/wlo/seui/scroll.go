package seui

import (
	"image"
	"math"
	"time"
)

// ScrollButton is TSe_ScrollButton (constructor FUN_004733b8): a track
// drawn from the control's image and a three-slice thumb from a separate
// sheet of three side-by-side states.
type ScrollButton struct {
	Component
	ThumbImg   int             // +0xd0
	Inset      int             // +0xd4 thumb inset along the track
	Vertical   bool            // +0xd8
	ThumbPos   int             // +0xdc
	dragFrom   int             // +0xe0
	Total      int             // +0xe4
	Visible_   int             // +0xe8 visible items
	Pos        int             // +0xec
	MaxPos     int             // +0xf0 largest thumb offset
	ThumbLen   int             // +0xf4
	TrackCap   int             // +0xf8
	ThumbCap   int             // +0xfc
	ThumbSrc   image.Rectangle // +0x100 x, +0x104 y, +0x108 w, +0x10c h
	dragging   bool            // +0x110
	dragX      int             // +0x114
	dragY      int             // +0x118
	pageDown   bool            // +0x11c
	pageUp     bool            // +0x11d
	lastPage   time.Time
	Interval   time.Duration // +0x128, 100 ms
	ThumbState byte          // +0x130
	OnChange   func()        // +0x148
	OnPageUp   func()        // +0x138
	OnPageDown func()        // +0x140
	// Now is the global tick (PTR_DAT_004c98e4); tests replace it.
	Now func() time.Time
}

func NewScrollButton(env *Env, owner Control) *ScrollButton {
	s := &ScrollButton{}
	s.initComponent(s, env, owner)
	s.Vertical = true
	s.Name = "TSe_ScrollButton"
	s.TabStop = false
	s.Interval = 100 * time.Millisecond
	s.Group, s.DownSound = 0xff, 0xff
	s.ThumbImg = -1
	s.Now = time.Now
	return s
}

// round is Delphi's Round: banker's rounding on the FPU.
func round(v float64) int { return int(math.RoundToEven(v)) }

// SetThumb is FUN_00473fac in the original argument order: sheet name,
// source x, height, width, source y.
func (s *ScrollButton) SetThumb(name string, x, h, w, y int) {
	s.ThumbSrc = image.Rect(x, y, x+w, y+h)
	s.ThumbImg = s.Env.Pics.Find(name)
}

// SetCaps is FUN_0047402c: thumb cap, then track cap.
func (s *ScrollButton) SetCaps(thumb, track int) { s.ThumbCap, s.TrackCap = thumb, track }

func (s *ScrollButton) length() int {
	if s.Vertical {
		return s.Height
	}
	return s.Width
}

// thumbFromPos is the thumb offset for the position (FUN_0047403c and the
// step functions).
func (s *ScrollButton) thumbFromPos() int {
	return round(float64(s.Pos) / float64(s.Total-s.Visible_) * float64(s.MaxPos))
}

// SetRange is FUN_00474150: total items and visible items.
func (s *ScrollButton) SetRange(total, visible int) {
	s.Total, s.Visible_ = total, visible
	if s.Total < visible {
		s.Total = visible
	}
	s.SetVertical(s.Vertical)
}

// SetVertical is FUN_00474264: thumb length and travel.
func (s *ScrollButton) SetVertical(v bool) {
	s.Vertical = v
	if s.Total != 0 {
		s.ThumbLen = round(float64(s.Visible_) / float64(s.Total) * float64(s.length()))
	}
	if s.ThumbLen < 2*s.ThumbCap {
		s.ThumbLen = 2 * s.ThumbCap
	}
	s.MaxPos = s.length() - s.ThumbLen
}

// SetPos is FUN_0047403c.
func (s *ScrollButton) SetPos(pos int) {
	s.Pos = pos
	if s.Total == s.Visible_ {
		s.ThumbPos = 0
		return
	}
	s.ThumbPos = s.thumbFromPos()
	s.changed()
}

func (s *ScrollButton) changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// StepUp is FUN_00473468.
func (s *ScrollButton) StepUp() {
	s.Pos = max(s.Pos-1, 0)
	s.ThumbPos = s.thumbFromPos()
	s.changed()
}

// StepDown is FUN_00473be4.
func (s *ScrollButton) StepDown() {
	s.Pos = min(s.Pos+1, s.Total-s.Visible_)
	s.ThumbPos = s.thumbFromPos()
	s.changed()
}

// PageDown is FUN_00473ca4.
func (s *ScrollButton) PageDown() {
	if s.Visible_ == s.Total {
		return
	}
	if s.Total-s.Visible_ < s.Pos+s.Visible_ {
		s.Pos = s.Total - s.Visible_
	} else {
		s.Pos += s.Visible_
	}
	s.ThumbPos = s.thumbFromPos()
	if s.OnPageDown != nil {
		s.OnPageDown()
	}
}

// PageUp is FUN_00473e9c.
func (s *ScrollButton) PageUp() {
	if s.Visible_ == s.Total {
		return
	}
	s.Pos = max(s.Pos-s.Visible_, 0)
	s.ThumbPos = s.thumbFromPos()
	if s.OnPageUp != nil {
		s.OnPageUp()
	}
}

// thumbRect is the thumb's screen rectangle (FUN_004732f8).
func (s *ScrollButton) thumbRect() image.Rectangle {
	a := s.Abs()
	if !s.Vertical {
		return image.Rect(a.X+s.ThumbPos, a.Y, a.X+s.ThumbPos+s.ThumbLen, a.Y+s.Height)
	}
	return image.Rect(a.X, a.Y+s.ThumbPos, a.X+s.Width, a.Y+s.ThumbPos+s.ThumbLen)
}

// HitTest is slot +0x68 (FUN_0047327c): the whole rectangle.
func (s *ScrollButton) HitTest(x, y int) bool { return image.Pt(x, y).In(s.Rect()) }

// LeftDown is slot +0x38 (FUN_00473544): on the thumb a drag starts;
// beside it a page direction is armed.
func (s *ScrollButton) LeftDown(_ byte, x, y int) {
	if !s.live() {
		return
	}
	a := s.Abs()
	if image.Pt(x, y).In(s.thumbRect()) {
		s.dragX, s.dragY, s.dragFrom, s.dragging = x, y, s.ThumbPos, true
		return
	}
	s.dragging, s.dragFrom = false, 0
	off := y - a.Y
	if !s.Vertical {
		off = x - a.X
		s.dragX = 0
	} else {
		s.dragY = 0
	}
	if off < s.ThumbPos {
		s.pageUp = true
	}
	if s.ThumbPos+s.ThumbLen < off {
		s.pageDown = true
	}
}

func (s *ScrollButton) release() {
	s.dragX, s.dragY, s.dragging, s.dragFrom = 0, 0, false, 0
	s.pageDown, s.pageUp = false, false
}

// LeftUp, LeftUpOutside and RightUpOutside (FUN_004736a8, FUN_00473708,
// FUN_00473768) end any drag or paging.
func (s *ScrollButton) LeftUp(shift byte, x, y int) {
	s.Component.LeftUp(shift, x, y)
	if s.live() {
		s.release()
	}
}

func (s *ScrollButton) LeftUpOutside(byte, int, int) {
	if s.live() {
		s.release()
	}
}

func (s *ScrollButton) RightUpOutside(byte, int, int) {
	if s.live() {
		s.release()
	}
}

// drag moves the thumb with the pointer (FUN_004737c8, FUN_0047398c).
func (s *ScrollButton) drag(x, y int) {
	if !s.live() || !s.dragging || s.Total == s.Visible_ || s.MaxPos == 0 {
		return
	}
	d := y - s.dragY
	if !s.Vertical {
		d = x - s.dragX
	}
	s.ThumbPos = min(max(d+s.dragFrom, 0), s.MaxPos)
	s.Pos = round(float64(s.ThumbPos) / float64(s.MaxPos) * float64(s.Total-s.Visible_))
	s.changed()
}

// MouseMove is slot +0x48; held is the left button.
func (s *ScrollButton) MouseMove(shift byte, x, y int) { s.drag(x, y) }

// CapturedMove is slot +0x4c.
func (s *ScrollButton) CapturedMove(shift byte, x, y int) { s.drag(x, y) }

// Wheel slots +0x54 and +0x58 (FUN_00473b74, FUN_00473bac).
func (s *ScrollButton) WheelDown() {
	if s.live() && s.Total != s.Visible_ {
		s.StepDown()
	}
}

func (s *ScrollButton) WheelUp() {
	if s.live() && s.Total != s.Visible_ {
		s.StepUp()
	}
}

// Update is slot +0x18 (FUN_00474354): paging repeats every interval while
// the pointer is held on the track, and the thumb state follows the
// pointer.
func (s *ScrollButton) Update(in *Input) {
	if !s.Visible {
		return
	}
	if s.self.HitTest(in.X, in.Y) {
		in.Hovered = s.self
	}
	if !s.Blocked() {
		s.Focused = in.Focused == s.self
	}
	onThumb := image.Pt(in.X, in.Y).In(s.thumbRect())
	if !onThumb && in.Hovered == s.self {
		if now := s.Now(); now.Sub(s.lastPage) > s.Interval {
			if s.pageDown {
				s.PageDown()
			}
			if s.pageUp {
				s.PageUp()
			}
			s.lastPage = now
		}
	} else {
		s.pageDown, s.pageUp = false, false
	}
	switch {
	case !onThumb && !s.pageDown && !s.pageUp:
		s.ThumbState = 0
	case !onThumb:
		s.ThumbState = 2
	case s.dragging:
		s.ThumbState = 2
	default:
		s.ThumbState = 1
	}
	s.self.Paint()
	s.updateChildren(in)
}

// Paint is slot +0x10 (FUN_00474318): track caps, then the track and thumb
// (FUN_004745fc).
func (s *ScrollButton) Paint() {
	if !s.Visible {
		return
	}
	a := s.Abs()
	pics, scr := s.Env.Pics, s.Env.Screen
	cw, ch := s.Clip.Dx(), s.Clip.Dy()
	tile := func(n, unit int, f func(off, size int)) {
		if unit <= 0 {
			return
		}
		for i := range n / unit {
			f(i*unit, unit)
		}
		if rem := n % unit; rem > 0 {
			f(n/unit*unit, rem)
		}
	}
	ts := s.ThumbSrc
	if !s.Vertical {
		// Left (FUN_004745a0) and right (FUN_00474d54) caps.
		pics.DrawRect(scr, s.Image, a.X, a.Y, image.Rect(0, 0, s.TrackCap, ch), true)
		pics.DrawRect(scr, s.Image, a.X+s.Width-s.TrackCap, a.Y, image.Rect(cw-s.TrackCap, 0, cw, ch), true)
		mid := cw - 2*s.TrackCap
		n := s.Width - 2*s.TrackCap
		for i := range max(n, 0) / max(mid, 1) {
			pics.DrawRect(scr, s.Image, a.X+s.TrackCap+mid*i, a.Y, image.Rect(s.Clip.Min.X+s.TrackCap, s.Clip.Min.Y, s.Clip.Min.X+s.TrackCap+mid, s.Clip.Min.Y+ch), true)
		}
		if mid > 0 {
			if rem := n % mid; rem > 0 {
				pics.DrawRect(scr, s.Image, a.X+s.Width-rem-s.TrackCap, a.Y, image.Rect(s.Clip.Min.X+s.TrackCap, s.Clip.Min.Y, s.Clip.Min.X+s.TrackCap+rem, s.Clip.Min.Y+ch), true)
			}
		}
		off := (ch - ts.Dy()) / 2
		sy := int(s.ThumbState)*ts.Dy() + ts.Min.Y
		x0 := a.X + s.ThumbPos + s.Inset
		pics.DrawRect(scr, s.ThumbImg, x0, a.Y+off, image.Rect(ts.Min.X, sy, ts.Min.X+s.ThumbCap, sy+ts.Dy()), true)
		tm := ts.Dx() - 2*s.ThumbCap
		tile(s.ThumbLen-2*s.ThumbCap, tm, func(o, size int) {
			pics.DrawRect(scr, s.ThumbImg, a.X+s.ThumbPos+s.ThumbCap+o, a.Y+off, image.Rect(ts.Min.X+s.ThumbCap, sy, ts.Min.X+s.ThumbCap+size, sy+ts.Dy()), true)
		})
		pics.DrawRect(scr, s.ThumbImg, a.X+s.ThumbPos+s.ThumbLen-s.ThumbCap-s.Inset, a.Y+off, image.Rect(ts.Min.X+ts.Dx()-s.ThumbCap, sy, ts.Min.X+ts.Dx(), sy+ts.Dy()), true)
		return
	}
	// Top (FUN_00474dc0) and bottom (FUN_00474538) caps.
	pics.DrawRect(scr, s.Image, a.X, a.Y, image.Rect(0, 0, cw, s.TrackCap), true)
	pics.DrawRect(scr, s.Image, a.X, a.Y+s.Height-s.TrackCap, image.Rect(0, ch-s.TrackCap, cw, ch), true)
	mid := ch - 2*s.TrackCap
	tile(s.Height-2*s.TrackCap, mid, func(o, size int) {
		y := a.Y + s.TrackCap + o
		if size < mid {
			y = a.Y + s.Height - size - s.TrackCap
		}
		pics.DrawRect(scr, s.Image, a.X, y, image.Rect(s.Clip.Min.X, s.Clip.Min.Y+s.TrackCap, s.Clip.Min.X+cw, s.Clip.Min.Y+s.TrackCap+size), true)
	})
	off := (cw - ts.Dx()) / 2
	sx := int(s.ThumbState)*ts.Dx() + ts.Min.X
	y0 := a.Y + s.ThumbPos
	pics.DrawRect(scr, s.ThumbImg, a.X+off, y0+s.Inset, image.Rect(sx, ts.Min.Y, sx+ts.Dx(), ts.Min.Y+s.ThumbCap), true)
	tm := ts.Dy() - 2*s.ThumbCap
	tile(s.ThumbLen-2*s.ThumbCap, tm, func(o, size int) {
		y := y0 + s.ThumbCap + o
		if size < tm {
			y = y0 + s.ThumbLen - size - s.ThumbCap
		}
		pics.DrawRect(scr, s.ThumbImg, a.X+off, y, image.Rect(sx, ts.Min.Y+s.ThumbCap, sx+ts.Dx(), ts.Min.Y+s.ThumbCap+size), true)
	})
	pics.DrawRect(scr, s.ThumbImg, a.X+off, y0+s.ThumbLen-s.ThumbCap-s.Inset, image.Rect(sx, ts.Min.Y+ts.Dy()-s.ThumbCap, sx+ts.Dx(), ts.Min.Y+ts.Dy()), true)
}
