package minigame

import (
	"fmt"
	"image"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

// Sheep ports Tsport_SheepDream: FUN_00138f3c, FUN_001391b0 and
// FUN_0013a28c. A six-way random dream cue is interpreted modulo four:
// hit odd cues, leave even cues alone. A missed odd cue or a hit on an even
// cue loses one of three lives. Survive thirty seconds to win.
type Sheep struct {
	Round
	Variant           byte
	Score, Lives      int
	cue               int
	answered          bool
	tick              time.Time
	frames, remaining int
	target            image.Rectangle
	NewMonster        func(uint16) Monster
	sleeper           Monster
}

const (
	sheepSeconds           = 30
	sheepCueFrames         = 25
	sheepDifficultySeconds = 6
	sheepCueKinds          = 6
	sheepDreamSheepNPC     = 17371
	sheepDreamPigNPC       = 17400
	sheepTargetLeft        = 307
	sheepTargetTop         = 234
	sheepTargetWidth       = 110
	sheepTargetHeight      = 97
)

func NewSheep(pics *picdb.DB, variant byte) *Sheep {
	if variant != 1 && variant != 2 {
		variant = 1
	}
	return &Sheep{Round: newRound(pics), Variant: variant, Lives: arcadeLives, remaining: sheepSeconds,
		target: image.Rect(sheepTargetLeft, sheepTargetTop, sheepTargetLeft+sheepTargetWidth, sheepTargetTop+sheepTargetHeight)}
}
func (g *Sheep) Pictures() []string {
	return []string{"59102", "SheepGameBG_2", "59087", "59090", "Sleepbabo", "sheepDreamLife", "Hammer_1", "SheepDream_1", "SheepDream_2", "SheepDream_3", "SheepDream_4", "SheepDream_5", "SheepDream_6", "SheepDream_7", "SheepDream_8", "SheepDream_9"}
}
func (g *Sheep) Begin(now time.Time) {
	if g.Started || g.Done() {
		return
	}
	g.Round.Begin(now)
	g.tick = now
	g.cue = 0
	if g.NewMonster != nil {
		id := uint16(sheepDreamSheepNPC)
		if g.Variant == 2 {
			id = sheepDreamPigNPC
		}
		g.sleeper = g.NewMonster(id)
	}
}
func (g *Sheep) Update(now time.Time) {
	if !g.ready(now) {
		return
	}
	end := g.startedAt.Add(sheepSeconds * time.Second)
	until := now
	if end.Before(until) {
		until = end
	}
	for !g.tick.Add(arcadeFrame).After(until) {
		g.tick = g.tick.Add(arcadeFrame)
		g.remaining = max(0, sheepSeconds-int(g.tick.Sub(g.startedAt)/time.Second))
		g.frames++
		if g.frames >= sheepCueFrames-(sheepSeconds-g.remaining)/sheepDifficultySeconds {
			if !g.answered && g.cue%2 == 1 {
				g.Lives--
			}
			g.cue, g.answered, g.frames = g.Rand(sheepCueKinds), false, 0
			if g.Lives <= 0 {
				g.finish(false, g.tick)
				return
			}
		}
	}
	if !now.Before(end) {
		g.remaining = 0
		g.finish(g.Lives > 0, end)
	}
}
func (g *Sheep) Click(x, y int, now time.Time) {
	g.Update(now)
	if !g.ready(now) || g.answered || !g.hitTarget(x, y) {
		return
	}
	g.answered = true
	if g.cue%2 == 1 {
		g.Score++
		g.sound("sound\\Wav1611.wav")
	} else {
		g.Lives--
		g.sound("sound\\wav1607.wav")
	}
	if g.Lives == 0 {
		g.finish(false, now)
	}
}
func (g *Sheep) Draw(dst *surface.Surface, _, _ int, _ time.Time) {
	dst.Fill(image.Rect(0, 0, dst.W, dst.H), arcadePanel)
	g.picture(dst, "59102", -50, -50)
	g.picture(dst, "59087", -50, 130)
	g.picture(dst, "59090", -50, -100)
	g.picture(dst, "SheepGameBG_2", 200, 130)
	arcadeText(dst, 30, 15, fmt.Sprintf("Dream  Lives: %d  Score: %d  Time: %d", g.Lives, g.Score, g.remaining))
	arcadeText(dst, 220, 490, "Hit matching dreams; avoid false dreams.")
	if !g.Started {
		return
	}
	if g.sleeper != nil {
		g.sleeper.Draw(dst, 515, 500, 26)
	}
	sleepX := 425
	if g.Variant == 2 {
		sleepX = 405
	}
	g.strip(dst, "Sleepbabo", sleepX, 300, 0, 2)
	index := (int(g.Variant)-1)*4 + 2
	switch g.cue % 4 {
	case 1:
		index = 3
	case 2:
		index += 2
	case 3:
		index += 3
	}
	frame := 0
	if g.answered {
		frame = 1
	}
	g.strip(dst, fmt.Sprintf("SheepDream_%d", index), 300, 220-max(0, 100-g.frames*20), frame, 2)
	dst.Frame(g.target, arcadeWhite)
	arcadeText(dst, 365, 350, fmt.Sprintf("Cue %d", g.cue+1))
	g.outcome(dst)
}

// The three native click regions from FUN_0013a28c cover the dream and actor.
func (g *Sheep) hitTarget(x, y int) bool {
	p := image.Pt(x, y)
	return p.In(g.target) || p.In(image.Rect(443, 199, 515, 256)) || p.In(image.Rect(482, 255, 507, 315))
}
