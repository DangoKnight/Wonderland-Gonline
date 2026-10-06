package minigame

import (
	"fmt"
	"image"
	"time"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

const (
	ScotdKind               = 2
	scotdWinKills           = 20
	scotdMaxEnemies         = 20
	scotdSpawnFrames        = 50
	scotdAIFrames           = 50
	scotdPlayerStep         = 5
	scotdEnemyStep          = 3
	scotdGroundY            = 500
	scotdLeft               = 90
	scotdRight              = 710
	scotdStrikeFrames       = 15
	scotdJumpFrames         = 50
	scotdInvulnerableFrames = 100
	scotdEnemyHitPoints     = 3
	scotdAttackReach        = 75
	scotdActorHalfWidth     = 25
	scotdActorHeight        = 80
	scotdHeroNPC            = 18032
	KeyLeft                 = 0x25
	KeyUp                   = 0x26
	KeyRight                = 0x27
	KeyDown                 = 0x28
	KeySpace                = 0x20
)

type scotdEnemy struct {
	x, y, direction, hp, jump int
	dead                      bool
	actor                     Monster
}

// Scotd ports the side-scrolling combat game. FUN_0017c0dc/0017c6f4,
// FUN_0017cc74/0017d05c/0017d39c: arrows move/jump/duck, space strikes,
// enemies patrol at 3 px/frame, 20 kills wins, and hits have 100-frame immunity.
// The native manager derives lives as 5 minus the difficulty byte.
type Scotd struct {
	Round
	NewMonster                             func(uint16) Monster
	Hero                                   Monster
	EnemyID                                uint16
	Lives, Kills                           int
	X, Y                                   int
	enemies                                []scotdEnemy
	frameAt                                time.Time
	frame, direction, strike, jump, immune int
	left, right, duck                      bool
}

func NewScotd(pics *picdb.DB, enemy uint16, difficulty byte) *Scotd {
	lives := 5 - int(difficulty)
	if lives < 1 || lives > 4 {
		lives = arcadeLives
	}
	return &Scotd{Round: newRound(pics), EnemyID: enemy, Lives: lives, X: 150, Y: scotdGroundY, direction: 1}
}
func (s *Scotd) Pictures() []string {
	return []string{"59082", "59087", "59090", "icon_heart", "S10151"}
}
func (s *Scotd) Begin(now time.Time) {
	if s.Started || s.Done() {
		return
	}
	s.Round.Begin(now)
	s.frameAt = now
	s.ResultDelay = 100 * arcadeFrame
	if s.NewMonster != nil {
		s.Hero = s.NewMonster(scotdHeroNPC)
	}
}
func (s *Scotd) Key(key int, down bool, now time.Time) bool {
	switch key {
	case KeyLeft, KeyRight, KeyUp, KeyDown, KeySpace:
	default:
		return false
	}
	s.Update(now)
	if !s.Started || s.Done() || !s.resultAt.IsZero() {
		return true
	}
	switch key {
	case KeyLeft:
		s.left = down
	case KeyRight:
		s.right = down
	case KeyDown:
		s.duck = down
	case KeyUp:
		if down && s.jump == 0 {
			s.jump = scotdJumpFrames
		}
	case KeySpace:
		if down && s.strike == 0 && !s.duck {
			s.strike = scotdStrikeFrames
			s.attack()
		}
	}
	return true
}
func (s *Scotd) Click(x, y int, now time.Time) {
	s.Update(now)
	if !s.ready(now) {
		return
	}
	if x < s.X {
		s.direction = -1
	} else {
		s.direction = 1
	}
	if s.strike == 0 {
		s.strike = scotdStrikeFrames
		s.attack()
	}
}
func (s *Scotd) attack() {
	for i := range s.enemies {
		e := &s.enemies[i]
		dx := e.x - s.X
		if e.dead || dx*s.direction < 0 || abs(dx) > scotdAttackReach || abs(e.y-s.Y) > scotdActorHeight {
			continue
		}
		e.hp--
		s.sound("sound\\SEB0027.wav")
		if e.hp == 0 {
			e.dead = true
			s.Kills++
		}
	}
}
func (s *Scotd) Update(now time.Time) {
	if !s.ready(now) {
		return
	}
	for !s.frameAt.Add(arcadeFrame).After(now) && s.resultAt.IsZero() {
		s.frameAt = s.frameAt.Add(arcadeFrame)
		s.frame++
		if s.frame%scotdSpawnFrames == 0 && len(s.enemies) < scotdMaxEnemies {
			e := scotdEnemy{x: scotdLeft + s.Rand(scotdRight-scotdLeft), y: scotdGroundY - s.Rand(scotdGroundY/3), direction: 1, hp: scotdEnemyHitPoints, jump: scotdJumpFrames / 2}
			if s.NewMonster != nil {
				e.actor = s.NewMonster(s.EnemyID)
			}
			s.enemies = append(s.enemies, e)
		}
		if s.left != s.right && !s.duck {
			if s.left {
				s.X -= scotdPlayerStep
				s.direction = -1
			} else {
				s.X += scotdPlayerStep
				s.direction = 1
			}
		}
		s.X = max(scotdLeft, min(scotdRight, s.X))
		if s.jump > 0 {
			s.jump--
			phase := scotdJumpFrames - s.jump
			s.Y = scotdGroundY - min(phase, s.jump)*scotdPlayerStep
		} else {
			s.Y = scotdGroundY
		}
		if s.strike > 0 {
			s.strike--
		}
		if s.immune > 0 {
			s.immune--
		}
		for i := range s.enemies {
			e := &s.enemies[i]
			if e.dead {
				continue
			}
			if s.frame%scotdAIFrames == 0 {
				if s.Rand(2) == 0 {
					e.direction = -1
				} else {
					e.direction = 1
				}
				if e.jump == 0 && s.Rand(100) > 50 {
					e.jump = scotdJumpFrames
				}
			}
			e.x += e.direction * scotdEnemyStep
			if e.x <= scotdLeft {
				e.x = scotdLeft
				e.direction = 1
			}
			if e.x >= scotdRight {
				e.x = scotdRight
				e.direction = -1
			}
			if e.jump > 0 {
				e.jump--
				e.y = scotdGroundY - min(scotdJumpFrames-e.jump, e.jump)*scotdPlayerStep
			} else {
				e.y = min(scotdGroundY, e.y+6)
			}
			if s.immune == 0 && !s.duck && s.strike == 0 && abs(e.x-s.X) < scotdActorHalfWidth*2 && abs(e.y-s.Y) < scotdActorHeight {
				s.Lives--
				s.immune = scotdInvulnerableFrames
			}
		}
		if s.Lives <= 0 || s.Kills >= scotdWinKills {
			s.finish(s.Lives > 0 && s.Kills >= scotdWinKills, s.frameAt)
		}
	}
	s.ready(now)
}
func (s *Scotd) Draw(dst *surface.Surface, _, _ int, now time.Time) {
	s.background(dst, "59082")
	for _, e := range s.enemies {
		if e.dead {
			continue
		}
		action := 2
		if e.direction > 0 {
			action = 6
		}
		if e.actor != nil {
			e.actor.Draw(dst, e.x, e.y, action)
		} else {
			dst.Fill(image.Rect(e.x-scotdActorHalfWidth, e.y-scotdActorHeight, e.x+scotdActorHalfWidth, e.y), 0xf800)
		}
	}
	action := 2
	if s.direction > 0 {
		action = 6
	}
	if s.strike > 0 {
		action += 32
	}
	if s.duck {
		action = 26
	}
	if s.Hero != nil {
		s.Hero.Draw(dst, s.X, s.Y, action)
	} else {
		dst.Fill(image.Rect(s.X-scotdActorHalfWidth, s.Y-scotdActorHeight, s.X+scotdActorHalfWidth, s.Y), 0x07e0)
	}
	arcadeText(dst, 100, 30, fmt.Sprintf("Lives %d   Defeated %d/%d", s.Lives, s.Kills, scotdWinKills))
	arcadeText(dst, 140, 560, "Arrows: move/jump/duck   Space or click: strike")
	s.outcome(dst)
}
