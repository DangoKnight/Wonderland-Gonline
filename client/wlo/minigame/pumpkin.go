package minigame

import (
	"fmt"
	"image"
	"math"
	"time"
	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
)

const (
	PumpkinKind           = 17
	pumpkinDuration       = 50 * time.Second
	pumpkinWinScore       = 30
	pumpkinGhostCount     = 10
	pumpkinSpawnFrames    = 20
	pumpkinGhostTurnY     = 230
	pumpkinGhostStartY    = 600
	pumpkinProjectileStep = 15
	pumpkinOriginX        = 400
	pumpkinOriginY        = 309
	pumpkinHitRadius      = 20
	pumpkinResultDelay    = 3 * time.Second
	pumpkinGhostNPC       = 15544
)

type pumpkinGhost struct {
	x, y                   int
	active, returning, hit bool
	actor                  Monster
}
type pumpkinShot struct {
	x, y, dx, dy float64
	active       bool
}

// Pumpkin ports FUN_001626bc/00162d94/001633c8/00163538: ten lanes,
// ghosts rise one pixel/frame then descend, one aimed projectile, three lives,
// 50 seconds and at least 30 hits to win. A missed returning ghost costs a life.
type Pumpkin struct {
	Round
	NewMonster        func(uint16) Monster
	DrawPlayer        func(*surface.Surface, int, int, int32)
	Score, Lives      int
	ghosts            [pumpkinGhostCount]pumpkinGhost
	shot              pumpkinShot
	frameAt, deadline time.Time
	frame             int
}

func NewPumpkin(pics *picdb.DB) *Pumpkin { return &Pumpkin{Round: newRound(pics), Lives: arcadeLives} }
func (p *Pumpkin) Pictures() []string {
	return []string{"59028", "59027", "icon_LD_GhostF", "icon_LD_GhostB", "icon_LD_GhostG", "S10416", "icon_Heart", "MiniGame_Score_1", "MiniGame_Time_1", "MiniGame_Life_1"}
}
func (p *Pumpkin) Begin(now time.Time) {
	if p.Started || p.Done() {
		return
	}
	p.Round.Begin(now)
	p.ResultDelay = pumpkinResultDelay
	p.frameAt = now.Add(arcadeCountdown)
	p.deadline = p.frameAt.Add(pumpkinDuration)
	for i := range p.ghosts {
		x := (i + 1) * 55
		if i >= 5 {
			x = 832 - (i-3)*55
		}
		p.ghosts[i] = pumpkinGhost{x: x, y: pumpkinGhostStartY}
		if p.NewMonster != nil {
			p.ghosts[i].actor = p.NewMonster(pumpkinGhostNPC)
		}
	}
}
func (p *Pumpkin) Click(x, y int, now time.Time) {
	p.Update(now)
	if !p.ready(now) || now.Before(p.startedAt.Add(arcadeCountdown)) || p.shot.active {
		return
	}
	dx, dy := float64(x-pumpkinOriginX), float64(y-pumpkinOriginY)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	p.shot = pumpkinShot{x: pumpkinOriginX, y: pumpkinOriginY, dx: dx / length * pumpkinProjectileStep, dy: dy / length * pumpkinProjectileStep, active: true}
}
func (p *Pumpkin) Update(now time.Time) {
	if !p.ready(now) {
		return
	}
	until := now
	if until.After(p.deadline) {
		until = p.deadline
	}
	for !p.frameAt.Add(arcadeFrame).After(until) && p.resultAt.IsZero() {
		p.frameAt = p.frameAt.Add(arcadeFrame)
		p.frame++
		if p.frame%pumpkinSpawnFrames == 0 {
			g := &p.ghosts[p.Rand(pumpkinGhostCount)]
			if !g.active {
				g.active = true
				g.hit = false
				g.returning = false
				g.y = pumpkinGhostStartY
			}
		}
		for i := range p.ghosts {
			g := &p.ghosts[i]
			if !g.active {
				continue
			}
			if g.hit {
				g.y += 8
			} else if g.returning {
				g.y++
			} else {
				g.y--
				if g.y < pumpkinGhostTurnY {
					g.returning = true
				}
			}
			if g.y > pumpkinGhostStartY {
				if !g.hit && g.returning {
					p.Lives--
				}
				g.active = false
			}
		}
		if p.shot.active {
			before := image.Pt(int(p.shot.x), int(p.shot.y))
			p.shot.x += p.shot.dx
			p.shot.y += p.shot.dy
			after := image.Pt(int(p.shot.x), int(p.shot.y))
			// Step is smaller than the native hit radius; include both endpoints.
			for i := range p.ghosts {
				g := &p.ghosts[i]
				if g.active && !g.hit && (math.Hypot(float64(g.x-after.X), float64(g.y-after.Y)) <= pumpkinHitRadius || math.Hypot(float64(g.x-before.X), float64(g.y-before.Y)) <= pumpkinHitRadius) {
					g.hit = true
					p.Score++
					p.shot.active = false
					p.sound("Sound\\Wav1611.wav")
					break
				}
			}
			if p.shot.x < 10 || p.shot.x > 800 || p.shot.y < 0 || p.shot.y > 600 {
				p.shot.active = false
			}
		}
		if p.Lives <= 0 {
			p.finish(false, p.frameAt)
		}
	}
	if p.resultAt.IsZero() && !now.Before(p.deadline) {
		p.finish(p.Score >= pumpkinWinScore && p.Lives > 0, p.deadline)
	}
	p.ready(now)
}
func (p *Pumpkin) Draw(dst *surface.Surface, _, _ int, now time.Time) {
	p.background(dst, "59028")
	for _, g := range p.ghosts {
		if !g.active {
			continue
		}
		action := 0
		if g.returning {
			action = 4
		}
		if g.hit {
			action = 26
		}
		if g.actor != nil {
			g.actor.Draw(dst, g.x, g.y, action)
		} else {
			if !p.picture(dst, "icon_LD_GhostF", g.x-15, g.y-76) {
				dst.Fill(image.Rect(g.x-15, g.y-35, g.x+15, g.y), 0xffff)
			}
		}
	}
	if p.DrawPlayer != nil {
		p.DrawPlayer(dst, pumpkinOriginX, pumpkinOriginY, 12)
	} else {
		arcadeText(dst, pumpkinOriginX-15, pumpkinOriginY, "Aim")
	}
	if p.shot.active {
		dst.Fill(image.Rect(int(p.shot.x)-4, int(p.shot.y)-4, int(p.shot.x)+4, int(p.shot.y)+4), 0xffe0)
	}
	seconds := max(0, int(p.deadline.Sub(now)/time.Second))
	arcadeText(dst, 50, 15, fmt.Sprintf("Lives %d   Hits %d/%d   Time %d", p.Lives, p.Score, pumpkinWinScore, seconds))
	if p.Started && now.Before(p.startedAt.Add(arcadeCountdown)) {
		arcadeText(dst, 350, 280, "Get ready")
	}
	p.outcome(dst)
}
