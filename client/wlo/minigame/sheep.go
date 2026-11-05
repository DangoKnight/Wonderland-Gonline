package minigame

import (
	"fmt"
	"image"
	"time"
	"wonderland-gonline/client/wlo/cursor"
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
	hammerAt          time.Time
}

const (
	DreamSheepVariant      byte = 1
	DreamPigVariant        byte = 2
	sheepSeconds                = 30
	sheepCueFrames              = 25
	sheepDifficultySeconds      = 6
	sheepCueKinds               = 6
	sheepDreamSheepNPC          = 17371
	sheepDreamPigNPC            = 17400
	sheepTargetLeft             = 307
	sheepTargetTop              = 234
	sheepTargetWidth            = 110
	sheepTargetHeight           = 97
	sheepHammerX                = 375
	sheepHammerY                = 200
	sheepHammerFrames           = 3
	sheepHammerBackTick         = 4
	sheepHammerHitTick          = 8
	sheepHammerResetTick        = 11
	sheepStandingHammer         = 0
	sheepRaisedHammer           = 1
	sheepStrikingHammer         = 2
	sheepSleeperX               = 475
	sheepSleeperY               = 550
	sheepSleepingAction         = 26
	sheepWakingAction           = 28
	sheepLifeSpacing            = 76
	sheepLifeWidth              = 66
	sheepLifeHeight             = 62
	sheepLifeY                  = 15
	sheepHUDY                   = 25
	sheepLifeLabelX             = 15
	sheepTimeLabelX             = 345
	sheepTimeDigitsX            = 445
	sheepScoreLabelX            = 630
	sheepScoreDigitsX           = 730
)

func NewSheep(pics *picdb.DB, variant byte) *Sheep {
	if variant != DreamSheepVariant && variant != DreamPigVariant {
		variant = DreamSheepVariant
	}
	return &Sheep{Round: newRound(pics), Variant: variant, Lives: arcadeLives, remaining: sheepSeconds,
		target: image.Rect(sheepTargetLeft, sheepTargetTop, sheepTargetLeft+sheepTargetWidth, sheepTargetTop+sheepTargetHeight)}
}

func (g *Sheep) Cursor() cursor.Shape {
	if g.Started && !g.finished && g.resultAt.IsZero() {
		return cursor.ShapeHammer
	}
	return cursor.ShapeNormal
}
func (g *Sheep) Pictures() []string {
	return []string{"59102", "SheepGameBG_2", "59087", "59090", "Sleepbabo", "sheepDreamLife", "sheepDreamLife_2", "Hammer_1", "MiniGame_Life_1", "MiniGame_Time_1", "MiniGame_Score_1", "Num_White5_1", "Word_Red_64L", "Word_Red_64U", "SheepDream_1", "SheepDream_2", "SheepDream_3", "SheepDream_4", "SheepDream_5", "SheepDream_6", "SheepDream_7", "SheepDream_8", "SheepDream_9"}
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
		if g.Variant == DreamPigVariant {
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
	if !g.ready(now) || !g.hitTarget(x, y) || g.hammerFrame(now) != sheepStandingHammer {
		return
	}
	g.hammerAt = now
	if g.answered {
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
func (g *Sheep) Draw(dst *surface.Surface, _, _ int, now time.Time) {
	dst.Fill(image.Rect(0, 0, dst.W, dst.H), arcadePanel)
	g.picture(dst, "59102", -50, -50)
	g.picture(dst, "59087", -50, 130)
	g.picture(dst, "59090", -50, -100)
	g.picture(dst, "SheepGameBG_2", 200, 130)
	g.picture(dst, "MiniGame_Life_1", sheepLifeLabelX, sheepHUDY)
	g.picture(dst, "MiniGame_Time_1", sheepTimeLabelX, sheepHUDY)
	g.picture(dst, "MiniGame_Score_1", sheepScoreLabelX, sheepHUDY)
	if g.Pics != nil {
		life := "sheepDreamLife"
		if g.Variant == DreamPigVariant {
			life = "sheepDreamLife_2"
		}
		if icon := g.Pics.Find(life); icon >= 0 {
			for i := 1; i <= g.Lives; i++ {
				g.Pics.DrawStretch(dst, icon, image.Rect(i*sheepLifeSpacing, sheepLifeY, i*sheepLifeSpacing+sheepLifeWidth, sheepLifeY+sheepLifeHeight), true)
			}
		}
		DrawNumber(dst, g.Pics, sheepTimeDigitsX, sheepHUDY, g.remaining, false, NumberSmall, "Num_White5_1")
		DrawNumber(dst, g.Pics, sheepScoreDigitsX, sheepHUDY, g.Score, false, NumberSmall, "Num_White5_1")
	}
	if !g.Started {
		return
	}
	if g.sleeper != nil {
		action := sheepSleepingAction
		if !g.resultAt.IsZero() && !g.win {
			action = sheepWakingAction
		}
		g.sleeper.Draw(dst, sheepSleeperX, sheepSleeperY, action)
	}
	sleepX := 425
	if g.Variant == DreamPigVariant {
		sleepX = 405
	}
	if g.resultAt.IsZero() || g.win {
		sleepY := 470
		if g.Variant == DreamPigVariant {
			sleepY = 480
		}
		g.strip(dst, "Sleepbabo", sleepX, sleepY, int(now.Sub(g.startedAt)/(time.Second/2))%2, 2)
	}
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
	// FUN_00139c60 draws a three-row hammer at a fixed point, independent
	// of mouse position. FUN_001391b0 selects rows on the 30 ms game clock.
	g.strip(dst, "Hammer_1", sheepHammerX, sheepHammerY, g.hammerFrame(now), sheepHammerFrames)
	if (!g.resultAt.IsZero() || g.finished) && g.Pics != nil {
		label := "You Lose"
		if g.win {
			label = "You Win"
		}
		DrawWord(dst, g.Pics, 400, 200, label, true, 65, 0, FontRed)
	}
}

func (g *Sheep) hammerFrame(now time.Time) int {
	if g.hammerAt.IsZero() {
		return sheepStandingHammer
	}
	if !g.resultAt.IsZero() && now.After(g.resultAt) {
		now = g.resultAt
	}
	tick := int(now.Sub(g.hammerAt) / arcadeFrame)
	switch {
	case tick >= sheepHammerResetTick:
		return sheepStandingHammer
	case tick >= sheepHammerHitTick:
		return sheepStrikingHammer
	case tick >= sheepHammerBackTick:
		return sheepRaisedHammer
	default:
		return sheepStandingHammer
	}
}

// The three native click regions from FUN_0013a28c cover the dream and actor.
func (g *Sheep) hitTarget(x, y int) bool {
	p := image.Pt(x, y)
	return p.In(g.target) || p.In(image.Rect(443, 199, 515, 256)) || p.In(image.Rect(482, 255, 507, 315))
}
