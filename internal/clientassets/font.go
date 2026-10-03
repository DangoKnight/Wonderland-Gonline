package clientassets

import (
	"fmt"

	"golang.org/x/text/encoding/traditionalchinese"
)

// Bitmap font layout of font/TATPC1.TWN. Every byte is XORed with 0x58. The
// file holds 256 single-byte glyphs (8x15, one byte per row) at c*15, then
// 13,910 Big5 glyphs (16x15, two big-endian bytes per row) at 3840+index*30.
// The index is the client's (aLogin FUN_00486e64, ca19ee build):
//
//	A440-C67E  0..5400      157 per lead byte
//	C940-F9FE  0x1519..     157 per lead byte
//	A140-A3FE  0x3326..     157 per lead byte
//	C6A1-C8FE  0x34be..     94 per lead byte
const (
	fontKey       = 0x58
	FontHeight    = 15
	fontASCIISize = 256 * FontHeight
	fontWideSize  = 2 * FontHeight
	fontBig5Count = 13910
	big5Trails    = 157
)

// Font is a decoded TATPC bitmap font.
type Font struct{ data []byte }

// DecodeFont decodes TATPC1.TWN.
func DecodeFont(raw []byte) (*Font, error) {
	if len(raw) != fontASCIISize+fontBig5Count*fontWideSize {
		return nil, fmt.Errorf("TATPC font: unexpected size %d", len(raw))
	}
	data := make([]byte, len(raw))
	for i, b := range raw {
		data[i] = b ^ fontKey
	}
	return &Font{data}, nil
}

// Glyph is a glyph's rows; bit 15 of each row is the leftmost pixel.
type Glyph struct {
	Width int
	Rows  [FontHeight]uint16
}

// Set reports whether pixel (x, y) is drawn.
func (g Glyph) Set(x, y int) bool { return g.Rows[y]>>(15-x)&1 != 0 }

// ASCII returns the 8-pixel glyph for a single-byte code.
func (f *Font) ASCII(c byte) Glyph {
	g := Glyph{Width: 8}
	for r := range FontHeight {
		g.Rows[r] = uint16(f.data[int(c)*FontHeight+r]) << 8
	}
	return g
}

// Big5 returns the 16-pixel glyph for a Big5 code, or false when the font has none.
func (f *Font) Big5(lead, trail byte) (Glyph, bool) {
	i := Big5Index(lead, trail)
	if i > MaxGlyph {
		return Glyph{}, false
	}
	return f.Wide(i), true
}

// Big5Index is FUN_00486e64's arithmetic: the glyph index of a lead and
// trail byte, masked to 16 bits as the caller stores it. The client does not
// validate; callers treat values above 0x3655 as "not a double-byte glyph".
func Big5Index(lead, trail byte) int {
	l, t := int(lead), int(trail)
	tt := t - 0x40
	if t > 0x7e {
		tt = t - 0x62
	}
	var i int
	switch {
	case l < 0x80:
		i = 0xffdc
	case l < 0xa4:
		i = (l-0xa1)*big5Trails + tt + 0x3326
	case l < 0xc6 || (l == 0xc6 && t < 0xa1):
		i = (l-0xa4)*big5Trails + tt
	case l < 0xc9:
		i = (l-0xc6)*0x5e + t - 0xa1 + 0x34be
	default:
		i = (l-0xc9)*big5Trails + 0x1519 + tt
	}
	return i & 0xffff
}

// MaxGlyph is the highest double-byte index the client draws (0x3655).
const MaxGlyph = 0x3655

// Wide returns the 16-pixel glyph at a double-byte index.
func (f *Font) Wide(i int) Glyph {
	g := Glyph{Width: 16}
	if i < 0 || i >= fontBig5Count {
		return g
	}
	o := fontASCIISize + i*fontWideSize
	for r := range FontHeight {
		g.Rows[r] = uint16(f.data[o+2*r])<<8 | uint16(f.data[o+2*r+1])
	}
	return g
}

// Glyphs splits Big5-encoded text into glyphs, as the client draws it.
func (f *Font) Glyphs(text []byte) []Glyph {
	var out []Glyph
	for i := 0; i < len(text); i++ {
		if c := text[i]; c >= 0x81 && i+1 < len(text) {
			if g, ok := f.Big5(c, text[i+1]); ok {
				out = append(out, g)
				i++
				continue
			}
		}
		out = append(out, f.ASCII(text[i]))
	}
	return out
}

// Big5Text encodes UTF-8 text in Big5, replacing unencodable runes with '?'.
func Big5Text(s string) []byte {
	var out []byte
	enc := traditionalchinese.Big5.NewEncoder()
	for _, r := range s {
		b, err := enc.Bytes([]byte(string(r)))
		if err != nil {
			b = []byte{'?'}
		}
		out = append(out, b...)
	}
	return out
}
