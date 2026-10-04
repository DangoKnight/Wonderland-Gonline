package minigame

import (
	"image"
	"time"

	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

// Effect is a game's one-shot animation slot (the object FUN_003c856c
// creates; FUN_003c9284 starts it, FUN_003c8830 draws and advances it):
// frames First..Last of a picture strip of Frames rows, one every Every,
// centred on (X, Y) with the frame's bottom on Y, drawn keyed or with the
// light blit at Level. Starting another replaces it.
type Effect struct {
	Picture     string
	Every       time.Duration
	Level       int // LevelKeyed for a plain keyed draw
	X, Y        int
	Frames      int
	First, Last int // 1-based rows
	step        int // +0x44
	at          time.Time
	active      bool
}

// LevelKeyed is the effect level drawn with the colour key alone (0xff).
const LevelKeyed = 0xff

// Play starts the effect.
func (e *Effect) Play(picture string, every time.Duration, level, x, y, frames, last, first int, now time.Time) {
	*e = Effect{Picture: picture, Every: every, Level: level, X: x, Y: y, Frames: max(frames, 1),
		First: first, Last: last, at: now, active: true}
}

// Active reports whether the effect still plays.
func (e *Effect) Active() bool { return e.active }

// Draw draws the current frame and advances; the effect ends after Last.
func (e *Effect) Draw(dst *surface.Surface, pics *picdb.DB, now time.Time) {
	if !e.active {
		return
	}
	i := pics.Find(e.Picture)
	if i < 0 {
		e.active = false
		return
	}
	w, h := pics.Size(i)
	fh := h / e.Frames
	row := e.First - 1 + e.step
	r := image.Rect(0, row*fh, w, (row+1)*fh)
	x, y := e.X-w/2, e.Y-fh
	if e.Level == LevelKeyed {
		pics.DrawRect(dst, i, x, y, r, true)
	} else {
		pics.DrawLight(dst, i, x, y, r, e.Level)
	}
	if now.Sub(e.at) >= e.Every {
		e.at = now
		e.step++
		if e.First+e.step > e.Last {
			e.active = false
		}
	}
}
