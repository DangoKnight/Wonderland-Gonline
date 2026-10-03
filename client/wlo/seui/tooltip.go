package seui

import (
	"image"

	"wonderland-go/client/wlo/surface"
)

// Tooltip layout from FUN_00477864.
const (
	tipCharsPerLine = 0x28
	tipCharPixels   = 8
	tipLinePixels   = 0x14
	tipAlpha        = 200
	tipTextWidth    = 0x140
	tipTextLine     = 0x10
	tipTextPaper    = 0x841
	tipTextInk      = 0xffff
	tipTextStyle    = 2
)

// Tooltip is FUN_00477864: a translucent box ending at (x, y) with the
// text inside, kept on screen. pen and fill are TColors.
func Tooltip(env *Env, x, y int, text []byte, pen, fill uint32) {
	if len(text) == 0 {
		return
	}
	n := len(text)
	w := n%tipCharsPerLine*tipCharPixels + 6
	h := (n/tipCharsPerLine+1)*tipLinePixels + 4
	r := image.Rect(x-w, y-h, x, y)
	if r.Max.X > 800 {
		r = r.Add(image.Pt(0x31e-r.Max.X, 0))
	}
	if r.Min.X < 0 {
		r = r.Add(image.Pt(2-r.Min.X, 0))
	}
	if r.Min.Y < 0 {
		r = r.Add(image.Pt(0, 0x32))
	}
	if r.Max.Y > 0x23a {
		r = r.Add(image.Pt(0, 0x23a-r.Max.Y))
	}
	scr := env.Screen
	scr.FillAlpha(r, fill, tipAlpha)
	scr.Frame(r, surface.TColor(pen))
	env.Text.Draw(r.Min.X+3, r.Min.Y+3, 0, false, true, scr, text, tipTextLine, tipTextWidth, tipTextPaper, tipTextInk, tipTextStyle)
}

// WrappedTooltip is FUN_00477a70: a translucent box at (x, y), n
// characters wide and as many lines of lineH as the text needs, kept on
// screen, with the text in white.
func WrappedTooltip(env *Env, x, y int, text []byte, pen, fill uint32, lineH, n int) {
	if len(text) == 0 || n <= 0 {
		return
	}
	w := n*tipCharPixels + 6
	h := (len(text)/n+1)*lineH + 4
	r := image.Rect(x, y, x+w, y+h)
	if r.Max.X > 800 {
		r = r.Add(image.Pt(800-r.Max.X-2, 0))
	}
	if r.Min.X < 0 {
		r = r.Add(image.Pt(2-r.Min.X, 0))
	}
	if r.Min.Y < 0 {
		r = r.Add(image.Pt(0, 2-r.Min.Y))
	}
	if lim := 600 - 0x1e; r.Max.Y > lim {
		r = r.Add(image.Pt(0, lim-r.Max.Y))
	}
	scr := env.Screen
	scr.FillAlpha(r, fill, tipAlpha)
	scr.Frame(r, surface.TColor(pen))
	env.Text.Draw(r.Min.X+3, r.Min.Y+3, 0, false, true, scr, text, lineH, n*tipCharPixels, tipTextPaper, tipTextInk, tipTextStyle)
}
