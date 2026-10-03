// Package text ports the client's bitmap text renderer (font object at
// PTR_DAT_004ca63c; WLRI build addresses throughout).
//
//	FUN_00489754  Draw: a Big5 string, wrapping at a width
//	FUN_00487a58  single-byte glyph cell
//	FUN_00487f14  double-byte glyph cell
//	FUN_00487358  ink/paper and the derived shadow colour
//	FUN_00486e64  double-byte glyph index (clientassets.Big5Index)
//
// The original renders each glyph into a cached 16-bit cell and blits the
// cell; drawing the cell's pixels directly is equivalent.
package text

import (
	"wonderland-go/client/wlo/surface"
	"wonderland-go/internal/clientassets"
)

// Edge is the outline colour (+0x587c), set to 0x1082 by the font
// constructor (0x486d90).
const Edge = 0x1082

// Renderer draws with one font. The traditional-Chinese client reads
// font\tatpc1.twn (Font_Create, 0x489ca0).
type Renderer struct {
	Font *clientassets.Font
}

// shadow is FUN_00487358's table: the shadow colour (+0x587e) for an ink.
func shadow(ink uint16) uint16 {
	switch ink {
	case 0xffff:
		return 0x8410
	case 0xf800:
		return 0xa800
	case 0x07e0:
		return 0x0540
	case 0x001f:
		return 0x0015
	case 0xf81f:
		return 0xa815
	case 0xfd55:
		return 0xad55
	case 0xffe0:
		return 0xad60
	case 0xfc00:
		return 0xaaa0
	case 0x07ff:
		return 0x0575
	case 0xc618, 0x8410:
		return 0x528a
	}
	return ink
}

// Draw is FUN_00489754. Arguments follow the original order: position,
// underline mode (1 every glyph, 2 all but spaces), bold (double-byte
// glyphs drawn twice), transparent blit, destination, Big5 text, line
// height, wrap width, paper (+0x587a) and ink (+0x5878) colours, and style:
// 0 plain, 1 outlined with 9-pixel advance, 2 embossed.
func (r *Renderer) Draw(x, y int, underline byte, bold, transparent bool, dst *surface.Surface,
	s []byte, lineHeight, width int, paper, ink uint16, style byte) {
	adv := 8
	if style == 1 {
		adv = 9
	}
	cx, cy := x, y
	for i := 1; i <= len(s); {
		idx := 0xffff
		if i != len(s) {
			idx = clientassets.Big5Index(s[i-1], s[i])
		}
		if idx > clientassets.MaxGlyph {
			if width < adv+cx-x {
				cy += lineHeight
				cx = x
			}
			c := s[i-1]
			r.cell(dst, cx, cy, r.Font.ASCII(c), transparent, paper, ink, style)
			if underline == 1 || (underline == 2 && c != ' ') {
				r.cell(dst, cx, cy+2, r.Font.ASCII('_'), transparent, paper, ink, style)
				r.cell(dst, cx+1, cy+2, r.Font.ASCII('_'), transparent, paper, ink, style)
			}
			i++
			cx += adv
			continue
		}
		if width < 2*adv+cx-x {
			cy += lineHeight
			cx = x
		}
		g := r.Font.Wide(idx)
		r.cell(dst, cx, cy, g, transparent, paper, ink, style)
		if bold {
			r.cell(dst, cx+1, cy, g, transparent, paper, ink, style)
		}
		if underline == 1 {
			for _, d := range []int{0, 4, 9} {
				r.cell(dst, cx+d, cy+2, r.Font.ASCII('_'), transparent, paper, ink, style)
			}
		}
		i += 2
		cx += 2 * adv
	}
}

// cell renders one glyph cell (FUN_00487a58 / FUN_00487f14) and blits it.
// Style 0 fills the glyph box with paper and sets glyph pixels to ink; style
// 1 fills a box two pixels larger with paper, shadows each glyph pixel to
// the right and below, edges the next pixels out, then sets the ink; style
// 2 starts from zero and edges each glyph pixel to the lower right. The
// blit covers 15 rows and the glyph width (plus one for single-byte or two
// for double-byte glyphs in style 1).
func (r *Renderer) cell(dst *surface.Surface, x, y int, g clientassets.Glyph, transparent bool, paper, ink uint16, style byte) {
	const rows = 17
	cols := g.Width + 2
	buf := make([]uint16, rows*cols)
	at := func(row, col int) *uint16 {
		if row < 0 || row >= rows || col < 0 || col >= cols {
			return new(uint16)
		}
		return &buf[row*cols+col]
	}
	switch style {
	case 0:
		for row := range clientassets.FontHeight {
			for col := range g.Width {
				*at(row, col) = paper
			}
		}
	case 1:
		for i := range buf {
			buf[i] = paper
		}
	}
	sh := shadow(ink)
	// The original walks rows and columns from the bottom right; later
	// writes win.
	for row := clientassets.FontHeight - 1; row >= 0; row-- {
		for col := g.Width - 1; col >= 0; col-- {
			if !g.Set(col, row) {
				continue
			}
			switch style {
			case 2:
				if p := at(row+1, col+1); *p != ink {
					*p = Edge
				}
			case 1:
				for _, d := range [][2]int{{0, 1}, {1, 1}, {1, 0}} {
					if p := at(row+d[0], col+d[1]); *p != ink {
						*p = sh
					}
				}
				for _, d := range [][2]int{{0, 2}, {1, 2}, {2, 2}, {2, 0}, {2, 1}} {
					if p := at(row+d[0], col+d[1]); *p == 0 {
						*p = Edge
					}
				}
			}
			*at(row, col) = ink
		}
	}
	w := g.Width
	if style == 1 {
		w += 2
		if g.Width == 8 {
			w = 9
		}
	}
	for row := range clientassets.FontHeight {
		dy := y + row
		if dy < 0 || dy >= dst.H {
			continue
		}
		for col := range w {
			dx := x + col
			if dx < 0 || dx >= dst.W {
				continue
			}
			v := buf[row*cols+col]
			if transparent && v == 0 {
				continue
			}
			dst.Pix[dy*dst.W+dx] = v
		}
	}
}
