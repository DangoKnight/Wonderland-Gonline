package app

import (
	"image"
	"time"
)

// The battle transitions are FUN_0049a2e8's modes 1 and 2, run each game
// frame by FUN_0049a484 after the scene is drawn (FUN_004a1d60).
//
// Legacy entry (mode 1) darkens the map before switching scenes. Gonline
// switches immediately to the battlefield and starts its reveal directly,
// avoiding a visible map battle cloud. Queued actions wait for the reveal.
//
// Leaving (mode 2): when the player's battle state clears (+0x1f16), the
// frame loop (0x4a2b12) starts mode 2 with FUN_0038fefc as its callback:
// the map is drawn again under black for steps 15 down to 1, and the
// battle is released at the end.
//
// Neither mode darkens map 11009 (0x2af1), though both still take their
// frames.
const (
	sceneFadeSteps     = 15
	sceneFadeAlphaStep = 12
	sceneFadeFrame     = fadeFrame
	sceneFadeUndimmed  = 11009 // "1F, Village Leader's"
)

const (
	sceneFadeNone         = 0
	sceneFadeEnterBattle  = 1
	sceneFadeLeaveBattle  = 2
	sceneFadeRevealBattle = 3 // Presentation continuation: reveal the newly loaded field.
)

type sceneFade struct {
	mode int
	step int // +0x531
	at   time.Time
}

func (c *Client) startSceneFade(mode int) {
	step := 1
	if mode == sceneFadeLeaveBattle || mode == sceneFadeRevealBattle {
		step = sceneFadeSteps
	}
	c.sceneFade = sceneFade{mode: mode, step: step, at: c.Now()}
}

// enteringBattle reports whether the map is still darkening before the
// battle field shows.
func (c *Client) enteringBattle() bool { return c.sceneFade.mode == sceneFadeEnterBattle }

// sceneFadeTick advances one step per game frame, also for undrawn
// background sessions.
func (c *Client) sceneFadeTick(now time.Time) {
	f := &c.sceneFade
	if f.mode == sceneFadeNone {
		return
	}
	for now.Sub(f.at) >= sceneFadeFrame && f.mode != sceneFadeNone {
		f.at = f.at.Add(sceneFadeFrame)
		switch {
		case f.mode == sceneFadeEnterBattle && f.step < sceneFadeSteps:
			f.step++
		case f.mode == sceneFadeEnterBattle:
			f.mode, f.step = sceneFadeRevealBattle, sceneFadeSteps
		case (f.mode == sceneFadeLeaveBattle || f.mode == sceneFadeRevealBattle) && f.step > 1:
			f.step--
		default:
			*f = sceneFade{}
		}
	}
}

// drawSceneFade darkens the scene just drawn.
func (c *Client) drawSceneFade() {
	f := c.sceneFade
	if f.mode == sceneFadeNone || c.World == nil || c.World.Player.Map == sceneFadeUndimmed {
		return
	}
	c.Screen.FillAlpha(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0, f.step*sceneFadeAlphaStep)
}
