// Package minigame ports the client's minigames (the sport manager
// TSportManage, PTR_DAT_004c9994): games the server starts with 57/1 and
// ends with 57/2, played on a full-screen picture of their own while the
// HUD is hidden. Each game reports a win or a loss, which the client sends
// back with 57/1.
package minigame

import (
	"image"

	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

// Number styles of FUN_003b9cb4: each digit advances by Advance, the glyph
// is drawn at the digit's slot (unstretched), and a centred number starts
// Half × digits to the left.
type numberStyle struct{ Advance, Half int }

var numberStyles = [...]numberStyle{
	{4, 2}, {8, 4}, {12, 6}, {16, 8}, {20, 10}, {26, 13}, {32, 16},
	{12, 12}, {20, 20}, {10, 10}, {10, 7}, {14, 7}, {11, 5},
}

// Number styles the minigames use.
const (
	NumberSmall = 3 // the clock and score digits
	NumberLarge = 5 // the countdown
)

// numberGlyphs is the digit count of a number font picture (0-9, stacked).
const numberGlyphs = 10

// DrawNumber is FUN_003b9c00: |value| in a picture font whose ten digits
// are stacked vertically, at (x, y), centred horizontally when centred.
func DrawNumber(dst *surface.Surface, pics *picdb.DB, x, y, value int, centred bool, style int, font string) {
	i := pics.Find(font)
	if i < 0 || style < 0 || style >= len(numberStyles) {
		return
	}
	if value < 0 {
		value = -value
	}
	digits := []byte(itoa(value))
	st := numberStyles[style]
	if centred {
		x -= len(digits) * st.Half
	}
	w, h := pics.Size(i)
	gh := h / numberGlyphs
	for k, d := range digits {
		row := int(d - '0')
		pics.DrawRect(dst, i, x+k*st.Advance, y, image.Rect(0, row*gh, w, (row+1)*gh), true)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for ; v > 0; v /= 10 {
		b = append([]byte{byte('0' + v%10)}, b...)
	}
	return string(b)
}

// Letter fonts of FUN_003ba370 (the table at DAT_0072a2d0): pairs of a
// lowercase and an uppercase picture of 26 glyphs stacked vertically.
var letterFonts = [...]string{"", "Word_Red_64L", "Word_Red_64U", "Word_Orange_56L", "Word_Orange_56U",
	"Word_Pink_55L", "Word_Pink_55U"}

const (
	FontRed      = 1
	letterGlyphs = 26
)

// DrawWord is FUN_003ba370: letters of text from the font pair starting at
// font (an uppercase letter uses the pair's uppercase picture), each
// advancing by (dx, dy), centred on (x, y) by half the total advance when
// centred; other characters leave a gap.
func DrawWord(dst *surface.Surface, pics *picdb.DB, x, y int, text string, centred bool, dx, dy, font int) {
	if font < 1 || font >= len(letterFonts) {
		font = 1
	}
	lower := font - (font+1)%2 // the pair's first entry (odd)
	n := len(text)
	if centred {
		x -= n * dx / 2
		y -= n * dy / 2
	}
	for k := 0; k < n; k++ {
		c := text[k]
		entry, row := 0, 0
		switch {
		case c >= 'A' && c <= 'Z':
			entry, row = lower+1, int(c-'A')
		case c >= 'a' && c <= 'z':
			entry, row = lower, int(c-'a')
		default:
			continue
		}
		i := pics.Find(letterFonts[entry])
		if i < 0 {
			continue
		}
		w, h := pics.Size(i)
		gh := h / letterGlyphs
		pics.DrawRect(dst, i, x+k*dx, y+k*dy, image.Rect(0, row*gh, w, (row+1)*gh), true)
	}
}

// rectRow is row r of a strip of w × h frames.
func rectRow(w, h, r int) image.Rectangle { return image.Rect(0, r*h, w, (r+1)*h) }
