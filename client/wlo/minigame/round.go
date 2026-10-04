package minigame

import (
	"image"
	"image/color"
	"math/rand/v2"
	"time"

	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"wonderland-go/client/wlo/cursor"
	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

const (
	arcadeFrame      = 30 * time.Millisecond
	arcadeCountdown  = 4 * time.Second
	arcadeResultWait = 5 * time.Second
	arcadeLives      = 3
	arcadeWhite      = uint16(0xffff)
	arcadePanel      = uint16(0x1082)
)

// Round owns the common Start/Leave/result lifecycle. Results are sent once;
// the manager keeps the game until the server's 57/2 acknowledgment.
type Round struct {
	Pics                *picdb.DB
	Rand                func(int) int
	Sound, Music        func(string)
	Result              func(bool)
	ResultDelay         time.Duration
	Started             bool
	finished            bool
	startedAt, resultAt time.Time
	win                 bool
}

func newRound(pics *picdb.DB) Round {
	return Round{Pics: pics, Rand: rand.IntN, ResultDelay: arcadeResultWait}
}
func (r *Round) Begin(now time.Time) {
	if r.Started || r.finished {
		return
	}
	r.Started, r.startedAt = true, now
	if r.Music != nil {
		r.Music("sound\\BGM0019.wav")
	}
}
func (r *Round) Running() bool        { return r.Started }
func (r *Round) Done() bool           { return r.finished }
func (r *Round) Cursor() cursor.Shape { return cursor.ShapeNormal }
func (r *Round) End(win bool) {
	if r.finished {
		return
	}
	r.finished, r.win = true, win
	if r.Result != nil {
		r.Result(win)
	}
}
func (r *Round) finish(win bool, now time.Time) {
	if r.resultAt.IsZero() {
		r.resultAt, r.win = now, win
	}
}
func (r *Round) ready(now time.Time) bool {
	if !r.Started || r.finished {
		return false
	}
	if !r.resultAt.IsZero() {
		if now.Sub(r.resultAt) >= r.ResultDelay {
			r.End(r.win)
		}
		return false
	}
	return true
}
func (r *Round) sound(path string) {
	if r.Sound != nil {
		r.Sound(path)
	}
}
func (r *Round) picture(dst *surface.Surface, name string, x, y int) bool {
	if r.Pics == nil {
		return false
	}
	i := r.Pics.Find(name)
	if i < 0 {
		return false
	}
	r.Pics.Draw(dst, i, x, y, true)
	return true
}
func (r *Round) background(dst *surface.Surface, name string) {
	dst.Fill(image.Rect(0, 0, dst.W, dst.H), arcadePanel)
	r.picture(dst, name, 0, 0)
}
func (r *Round) outcome(dst *surface.Surface) {
	if r.resultAt.IsZero() && !r.finished {
		return
	}
	label := "You Lose"
	if r.win {
		label = "You Win"
	}
	arcadeText(dst, 350, 280, label)
}

// arcadeText renders fallback labels and controls when a native bitmap is absent.
// Game artwork still comes from the shared picture database.
func arcadeText(dst *surface.Surface, x, y int, text string) {
	pen := fixed.P(x, y+basicfont.Face7x13.Ascent)
	for _, ch := range text {
		box, mask, at, advance, ok := basicfont.Face7x13.Glyph(pen, ch)
		if ok {
			origin := box.Min
			box = box.Intersect(image.Rect(0, 0, dst.W, dst.H))
			for py := box.Min.Y; py < box.Max.Y; py++ {
				for px := box.Min.X; px < box.Max.X; px++ {
					if color.AlphaModel.Convert(mask.At(at.X+px-origin.X, at.Y+py-origin.Y)).(color.Alpha).A > 0 {
						dst.Pix[py*dst.W+px] = arcadeWhite
					}
				}
			}
		}
		pen.X += advance
	}
}

// strip draws one row from a native vertical animation sheet.
func (r *Round) strip(dst *surface.Surface, name string, x, y, frame, rows int) bool {
	if r.Pics == nil {
		return false
	}
	i := r.Pics.Find(name)
	if i < 0 {
		return false
	}
	w, h := r.Pics.Size(i)
	if rows <= 0 || h/rows == 0 {
		return false
	}
	h /= rows
	frame = max(0, min(rows-1, frame))
	r.Pics.DrawRect(dst, i, x, y, image.Rect(0, frame*h, w, (frame+1)*h), true)
	return true
}
