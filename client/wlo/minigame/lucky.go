package minigame

import (
	"fmt"
	"image"
	"time"
	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

// Lucky is TLuckey (FUN_0016c4fc, FUN_0016ce44, FUN_0016ceb0,
// FUN_0016d170, FUN_0016d3f4 and FUN_0016d4c8). Follow the pointer to catch
// falling rewards and avoid bombs. Thirty catches in thirty seconds with
// at least one life remaining wins. The emitter moves seven pixels per
// native 30ms frame; the player moves at most ten; drops fall by ten.
type Lucky struct {
	Round
	Score, Lives               int
	x, aim, emitter, direction int
	tick                       time.Time
	drops                      []luckyDrop
	seconds                    int
	lastDropX                  int
	DrawPlayer                 func(*surface.Surface, int, int, int32)
}
type luckyDrop struct {
	X, Y int
	Bomb bool
}

const (
	luckySeconds        = 30
	luckyWinScore       = 30
	luckyPlayerY        = 550
	luckyEmitterY       = 150
	luckyLeft           = 30
	luckyRight          = 770
	luckyPlayerStep     = 10
	luckyEmitterStep    = 7
	luckyFallStep       = 10
	luckyDropKinds      = 15
	luckyBombKind       = 2
	luckyMaxDrops       = 51
	luckyCatchHalfWidth = 30
	luckyCatchTop       = 50
	luckyCatchBottom    = 20
)

func NewLucky(pics *picdb.DB) *Lucky {
	r := newRound(pics)
	r.ResultDelay = 3 * time.Second
	return &Lucky{Round: r, Lives: arcadeLives, x: 400, aim: 400, emitter: 400, direction: 1, seconds: luckySeconds}
}
func (g *Lucky) Pictures() []string {
	return []string{"59001", "59016", "59083", "2302", "S10816L", "S10152", "MiniGame_Score_1", "MiniGame_Time_1"}
}
func (g *Lucky) Begin(now time.Time) {
	if g.Started || g.Done() {
		return
	}
	g.Round.Begin(now)
	g.tick = now.Add(arcadeCountdown)
}
func (g *Lucky) Aim(x, y int) { g.aim = max(luckyLeft, min(luckyRight, x)) }
func (g *Lucky) Update(now time.Time) {
	if !g.ready(now) || now.Before(g.startedAt.Add(arcadeCountdown)) {
		return
	}
	end := g.startedAt.Add(arcadeCountdown + luckySeconds*time.Second)
	until := now
	if end.Before(until) {
		until = end
	}
	g.seconds = max(0, luckySeconds-int(until.Sub(g.startedAt.Add(arcadeCountdown))/time.Second))
	for !g.tick.Add(arcadeFrame).After(until) {
		g.tick = g.tick.Add(arcadeFrame)
		g.x += max(-luckyPlayerStep, min(luckyPlayerStep, g.aim-g.x))
		g.emitter += g.direction * luckyEmitterStep
		if g.emitter < luckyLeft {
			g.direction = 1
		}
		if g.emitter > luckyRight {
			g.direction = -1
		}
		alive := g.drops[:0]
		for _, d := range g.drops {
			d.Y += luckyFallStep
			w, h, _ := g.dropSize(d.Bomb)
			if abs(d.X-g.x) < w/2+10 && d.Y > luckyPlayerY-h-luckyCatchTop && d.Y < luckyPlayerY+luckyCatchBottom {
				if d.Bomb {
					g.Lives--
					g.sound("sound\\SEB0036.wav")
				} else {
					g.Score++
					g.sound("sound\\wav0152.wav")
				}
				continue
			}
			if d.Y <= luckyPlayerY {
				alive = append(alive, d)
			}
		}
		g.drops = alive
		if g.Lives <= 0 {
			g.Lives = 0
			g.finish(false, g.tick)
			return
		}
		kind := g.Rand(luckyDropKinds)
		if kind <= luckyBombKind && len(g.drops) < luckyMaxDrops {
			w, _, _ := g.dropSize(kind == luckyBombKind)
			spaced := g.lastDropX == 0 || abs(g.emitter-g.lastDropX) > w
			if spaced {
				g.lastDropX = g.emitter
				g.drops = append(g.drops, luckyDrop{X: g.emitter, Y: luckyEmitterY - luckyCatchTop, Bomb: kind == luckyBombKind})
			}
		}
	}
	if !now.Before(end) {
		g.finish(g.Score >= luckyWinScore && g.Lives > 0, end)
	}
}
func (g *Lucky) Click(x, y int, now time.Time) { g.Aim(x, y) }
func (g *Lucky) Draw(dst *surface.Surface, mx, my int, now time.Time) {
	dst.Fill(image.Rect(0, 0, dst.W, dst.H), arcadePanel)
	g.picture(dst, "59001", 0, -50)
	g.picture(dst, "59016", 0, 0)
	g.picture(dst, "59083", 0, 420)
	arcadeText(dst, 30, 15, fmt.Sprintf("Lucky  Lives: %d  Caught: %d/%d  Time: %d", g.Lives, g.Score, luckyWinScore, g.seconds))
	arcadeText(dst, 220, 580, "Move the pointer to catch falling rewards. Avoid bombs.")
	if !g.Started {
		return
	}
	if g.DrawPlayer != nil {
		action := int32(12)
		if g.aim < g.x-luckyPlayerStep {
			action = 2
		}
		if g.aim > g.x+luckyPlayerStep {
			action = 6
		}
		g.DrawPlayer(dst, g.x, luckyPlayerY, action)
	} else {
		dst.Frame(image.Rect(g.x-luckyCatchHalfWidth, luckyPlayerY-luckyCatchTop, g.x+luckyCatchHalfWidth, luckyPlayerY+luckyCatchBottom), arcadeWhite)
	}
	for _, d := range g.drops {
		name := "2302"
		if d.Bomb {
			name = "S10816L"
		}
		w, h, rows := g.dropSize(d.Bomb)
		frame := int(now.Sub(g.startedAt)/arcadeFrame) % rows
		if !g.strip(dst, name, d.X-w/2, d.Y-h, frame, rows) {
			arcadeText(dst, d.X, d.Y, map[bool]string{false: "+", true: "B"}[d.Bomb])
		}
	}
	g.outcome(dst)
}

func (g *Lucky) dropSize(bomb bool) (w, h, rows int) {
	name, rows := "2302", 1
	if bomb {
		name, rows = "S10816L", 2
	}
	w, h = 40, 30
	if g.Pics != nil {
		if i := g.Pics.Find(name); i >= 0 {
			w, h = g.Pics.Size(i)
			h /= rows
		}
	}
	return
}
