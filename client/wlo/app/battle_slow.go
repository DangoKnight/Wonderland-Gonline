package app

import (
	"image"
	"time"

	clientbattle "wonderland-gonline/client/wlo/battle"
)

const (
	battleSlowPicture       = "E119" // Native initializer at 0x0038c182 loads the four-frame turtle sheet.
	battleSlowFrames        = 4
	battleSlowFrameDuration = 250 * time.Millisecond // FUN_00391f18.
	battleSlowWarnSeconds   = 10                     // FUN_0039800c enables +6 when secondary countdown reaches ten.
	battleSlowOffsetX       = 20
	battleSlowMinY          = 90 // FUN_00398524 keeps the turtle above short fighters.
)

// FUN_00398524 shows turtles for living player/pet fighters whose AC53:5
// submission receipt has not arrived. The secondary timer continues after our
// own choices, so slow teammates remain visible until playback begins.
func (c *Client) battleSlowFrame(f clientbattle.Fighter, now time.Time) int {
	v := &c.battle
	if !v.state.Active || !v.state.Ready || v.state.Finished || v.waitStart.IsZero() || c.battleAnimating() ||
		f.Kind != clientbattle.Player && f.Kind != clientbattle.Pet || f.HP == 0 || f.Removed || f.Waiting || f.Submitted ||
		c.hotbar.submitted[[2]byte{f.X, f.Y}] {
		return -1
	}
	elapsed := now.Sub(v.waitStart)
	limit := v.state.TurnLimit()
	if elapsed < time.Duration(limit-battleSlowWarnSeconds)*time.Second || elapsed >= time.Duration(limit+1)*time.Second {
		return -1
	}
	return int(elapsed/battleSlowFrameDuration) % battleSlowFrames
}

func (c *Client) drawBattleSlow(f clientbattle.Fighter, b *battleBody, at image.Point, now time.Time) {
	frame := c.battleSlowFrame(f, now)
	if frame < 0 {
		return
	}
	i := c.Pics.Find(battleSlowPicture)
	w, h := c.Pics.Size(i)
	h /= battleSlowFrames
	if w == 0 || h == 0 {
		return
	}
	c.Pics.DrawRect(c.Screen, i, at.X-w/2+battleSlowOffsetX, max(battleSlowMinY, at.Y-b.height),
		image.Rect(0, frame*h, w, (frame+1)*h), true)
}
