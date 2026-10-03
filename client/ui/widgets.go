package ui

import (
	"image"
	"image/color"
	"math"
)

// Input is one frame of pointer and keyboard state.
type Input struct {
	X, Y        int
	Down        bool // left button held
	Pressed     bool // left button went down this frame
	Released    bool // left button went up this frame
	DoubleClick bool // second press of a double click this frame
	Text        []byte
	Keys        []Key
}

type Key int

const (
	KeyEnter Key = iota + 1
	KeyBackspace
	KeyTab
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyDelete
	KeyEscape
)

func (in *Input) at() image.Point { return image.Pt(in.X, in.Y) }

func rgb565(c uint16) color.RGBA {
	r, g, b := byte(c>>11), byte(c>>5&63), byte(c&31)
	return color.RGBA{r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2, 255}
}

// tColor converts a Delphi TColor ($00BBGGRR).
func tColor(c uint32) color.RGBA { return color.RGBA{byte(c), byte(c >> 8), byte(c >> 16), 255} }

// button is TSe_FixedButton: frame 0 idle, 1 under the pointer, 2 held. A
// click fires on release while still held over the button (FUN_0046bc14,
// FUN_0046c01c, FUN_0046bbc4).
type button struct {
	sprite  string
	at      image.Point
	frame   int
	held    bool
	onClick func()
}

func (b *button) rect(a *Assets) image.Rectangle {
	s, err := a.Skin(b.sprite, 3)
	if err != nil {
		return image.Rectangle{}
	}
	w, h := s.Size()
	return image.Rect(b.at.X, b.at.Y, b.at.X+w, b.at.Y+h)
}

func (b *button) update(a *Assets, in *Input) {
	over := in.at().In(b.rect(a))
	switch {
	case over && in.Pressed:
		b.held, b.frame = true, 2
	case over && in.Released && b.held:
		b.held, b.frame = false, 1
		if b.onClick != nil {
			b.onClick()
		}
	case over:
		if b.frame != 2 {
			b.frame = 1
		}
	default:
		b.held, b.frame = false, 0
	}
}

func (b *button) draw(a *Assets, dst *image.RGBA) error {
	s, err := a.Skin(b.sprite, 3)
	if err != nil {
		return err
	}
	s.Draw(dst, b.at.X, b.at.Y, b.frame)
	return nil
}

// listItem is one TSe_SelectText row; color 0 uses the list colour.
type listItem struct {
	text  []byte
	color uint16
}

// selectList is TSe_SelectText as configured by the server form: 20-pixel
// rows, a selected row filled with the selection colour, text inset 2 px.
type selectList struct {
	bounds    image.Rectangle
	items     []listItem
	top       int
	selected  int
	textColor uint16
	fill      color.RGBA
	onSelect  func(int)
	onChoose  func(int)
	scroll    *scrollBar
}

const listRowHeight = 20

func (l *selectList) visible() int { return l.bounds.Dy() / listRowHeight }

// rowAt is FUN_0046fb58's hover row: the visible row under the pointer, or -1.
func (l *selectList) rowAt(p image.Point) int {
	if !p.In(l.bounds) {
		return -1
	}
	row := (p.Y - l.bounds.Min.Y) / listRowHeight
	if row >= l.visible() || l.top+row >= len(l.items) {
		return -1
	}
	return l.top + row
}

func (l *selectList) set(items []listItem) {
	l.items, l.top, l.selected = items, 0, -1
	l.syncScroll()
}

func (l *selectList) scrollBy(d int) {
	l.top = max(0, min(l.top+d, len(l.items)-l.visible()))
	l.syncScroll()
}

func (l *selectList) syncScroll() {
	if l.scroll != nil {
		l.scroll.setRange(len(l.items), l.visible(), l.top)
	}
}

func (l *selectList) update(in *Input) {
	i := l.rowAt(in.at())
	if i < 0 {
		return
	}
	if in.Released {
		l.selected = i
		if l.onSelect != nil {
			l.onSelect(i)
		}
	}
	// FUN_0046ef5c: the second callback is assumed to be the double-click
	// event; its dispatch path has not been traced.
	if in.DoubleClick && l.onChoose != nil {
		l.onChoose(i)
	}
}

func (l *selectList) draw(a *Assets, dst *image.RGBA) {
	for row := range l.visible() {
		i := l.top + row
		if i >= len(l.items) {
			break
		}
		y := l.bounds.Min.Y + row*listRowHeight
		if i == l.selected {
			fillRect(dst, image.Rect(l.bounds.Min.X, y-2, l.bounds.Max.X, y-2+listRowHeight), l.fill)
		}
		c := l.items[i].color
		if c == 0 {
			c = l.textColor
		}
		a.Text(dst, l.bounds.Min.X+2, y, l.items[i].text, rgb565(c))
	}
}

// scrollBar is TSe_ScrollButton (vertical): a three-slice thumb from a
// three-state sheet of side-by-side frames, centred in the control. Thumb
// geometry follows FUN_00474150/FUN_0047403c/FUN_00474264.
type scrollBar struct {
	at                 image.Point
	width, length      int // control width and track length
	sheet              string
	frameW, frameH     int // one thumb frame
	capLen             int
	frame              int
	thumbLen, thumbPos int
}

func (s *scrollBar) setRange(total, visible, top int) {
	total = max(total, visible)
	s.thumbLen = s.length
	if total != 0 {
		s.thumbLen = int(math.RoundToEven(float64(visible) / float64(total) * float64(s.length)))
	}
	s.thumbLen = max(s.thumbLen, 2*s.capLen)
	s.thumbPos = 0
	if total != visible {
		s.thumbPos = int(math.RoundToEven(float64(top) / float64(total-visible) * float64(s.length-s.thumbLen)))
	}
}

func (s *scrollBar) draw(a *Assets, dst *image.RGBA) error {
	sheet, err := a.Skin(s.sheet, 1)
	if err != nil {
		return err
	}
	x := s.at.X + (s.width-s.frameW)/2
	y := s.at.Y + s.thumbPos
	sx := s.frame * s.frameW
	blit := func(srcY, h, dy int) { sheet.DrawRect(dst, x, dy, image.Rect(sx, srcY, sx+s.frameW, srcY+h)) }
	blit(0, s.capLen, y)
	mid := s.frameH - 2*s.capLen
	span := s.thumbLen - 2*s.capLen
	for i := range span / mid {
		blit(s.capLen, mid, y+s.capLen+i*mid)
	}
	if r := span % mid; r > 0 {
		blit(s.capLen, r, y+s.thumbLen-r-s.capLen)
	}
	blit(s.frameH-s.capLen, s.capLen, y+s.thumbLen-s.capLen)
	return nil
}

// editor is a single-line TSe_Editor field over a Panel26 frame.
type editor struct {
	at       image.Point
	width    int
	text     []byte
	max      int
	password bool
	onEnter  func()
}

func (e *editor) update(in *Input) {
	for _, c := range in.Text {
		if len(e.text) < e.max && c >= 0x20 && c < 0x7f {
			e.text = append(e.text, c)
		}
	}
	for _, k := range in.Keys {
		switch k {
		case KeyBackspace:
			if len(e.text) > 0 {
				e.text = e.text[:len(e.text)-1]
			}
		case KeyEnter:
			if e.onEnter != nil {
				e.onEnter()
			}
		}
	}
}

func (e *editor) draw(a *Assets, dst *image.RGBA, focused, caret bool) error {
	panel, err := a.Skin("Panel26.bmp", 1)
	if err != nil {
		return err
	}
	// Init clip (0,0,0x78,0x15), height 0xc, margins 1 (constructor +0x70).
	ninePatch(panel, dst, image.Rect(e.at.X, e.at.Y, e.at.X+e.width, e.at.Y+0xc), image.Rect(0, 0, 0x78, 0x15), 1, 1, 1, 1)
	shown := e.text
	if e.password {
		shown = bytesRepeat('*', len(e.text))
	}
	w := a.Text(dst, e.at.X+2, e.at.Y, shown, rgb565(0x0841))
	if focused && caret {
		c, err := a.Skin("InputCursor.bmp", 1)
		if err != nil {
			return err
		}
		// The capture shows 15 of InputCursor.bmp's 16 rows.
		c.DrawRect(dst, e.at.X+3+w, e.at.Y-1, image.Rect(0, 0, 3, 15))
	}
	return nil
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// ninePatch is TSe_panel's frame (FUN_0046b11c corners, FUN_0046b96c and
// FUN_0046af8c top/bottom, FUN_0046b2dc and FUN_0046b7f8 sides, FUN_0046b438
// centre): the source clip is cut by margins, edges and centre are tiled
// whole and then by the remainder. Source rectangles are cut to the image.
func ninePatch(s *Sprite, dst *image.RGBA, dest image.Rectangle, clip image.Rectangle, l, t, r, b int) {
	cw, ch := clip.Dx()-l-r, clip.Dy()-t-b
	w, h := dest.Dx()-l-r, dest.Dy()-t-b
	src := func(x0, y0, x1, y1 int) image.Rectangle {
		return image.Rect(clip.Min.X+x0, clip.Min.Y+y0, clip.Min.X+x1, clip.Min.Y+y1)
	}
	put := func(dx, dy int, sr image.Rectangle) { s.DrawRect(dst, dest.Min.X+dx, dest.Min.Y+dy, sr) }
	// tile repeats a source span across n pixels: whole copies, then the remainder.
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
	tile(w, cw, func(x, sw int) {
		tile(h, ch, func(y, sh int) { put(l+x, t+y, src(l, t, l+sw, t+sh)) })
		put(l+x, 0, src(l, 0, l+sw, t))
		put(l+x, dest.Dy()-b, src(l, clip.Dy()-b, l+sw, clip.Dy()))
	})
	tile(h, ch, func(y, sh int) {
		put(0, t+y, src(0, t, l, t+sh))
		put(dest.Dx()-r, t+y, src(clip.Dx()-r, t, clip.Dx(), t+sh))
	})
	put(0, 0, src(0, 0, l, t))
	put(dest.Dx()-r, 0, src(clip.Dx()-r, 0, clip.Dx(), t))
	put(dest.Dx()-r, dest.Dy()-b, src(clip.Dx()-r, clip.Dy()-b, clip.Dx(), clip.Dy()))
	put(0, dest.Dy()-b, src(0, clip.Dy()-b, l, clip.Dy()))
}
