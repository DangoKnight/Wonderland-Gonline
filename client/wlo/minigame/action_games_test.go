package minigame

import (
	"testing"
	"time"
)

func TestScotdControlsAndCombat(t *testing.T) {
	now := time.Unix(100, 0)
	s := NewScotd(nil, 14605, 1)
	s.Rand = func(int) int { return 0 }
	if s.Lives != 4 {
		t.Fatalf("lives %d", s.Lives)
	}
	s.Begin(now)
	startX := s.X
	if s.Key('A', true, now) {
		t.Fatal("unrelated key consumed")
	}
	s.Key(KeyRight, true, now)
	s.Update(now.Add(3 * arcadeFrame))
	if s.X != startX+15 {
		t.Fatalf("position %d", s.X)
	}
	s.Key(KeyRight, false, now.Add(3*arcadeFrame))
	s.Key(KeyUp, true, now.Add(3*arcadeFrame))
	s.Update(now.Add(4 * arcadeFrame))
	if s.Y >= scotdGroundY {
		t.Fatal("jump did not leave ground")
	}
	s.enemies = []scotdEnemy{{x: s.X + 20, y: s.Y, hp: 1}, {x: s.X - 20, y: s.Y, hp: 1}}
	s.Key(KeySpace, true, now.Add(4*arcadeFrame))
	if s.Kills != 1 || !s.enemies[0].dead || s.enemies[1].dead {
		t.Fatal("strike direction or defeat accounting")
	}
}

func TestScotdSpawnAndResult(t *testing.T) {
	now := time.Unix(100, 0)
	s := NewScotd(nil, 14605, 2)
	s.Rand = func(int) int { return 0 }
	s.Begin(now)
	s.duck = true // Avoid contacts while checking native spawn cadence.
	s.Update(now.Add(49 * arcadeFrame))
	if len(s.enemies) != 0 {
		t.Fatal("early spawn")
	}
	s.Update(now.Add(50 * arcadeFrame))
	if len(s.enemies) != 1 || s.enemies[0].hp != 3 {
		t.Fatal("native spawn cadence or HP")
	}
	s.Update(now.Add(2000 * arcadeFrame))
	if len(s.enemies) != 20 {
		t.Fatalf("enemy limit %d", len(s.enemies))
	}
	calls := 0
	s.Result = func(win bool) {
		calls++
		if !win {
			t.Fatal("victory reported loss")
		}
	}
	s.Kills = 20
	finish := s.frameAt.Add(arcadeFrame)
	s.Update(finish)
	s.Update(finish.Add(99 * arcadeFrame))
	if calls != 0 {
		t.Fatal("early result")
	}
	s.Update(finish.Add(100 * arcadeFrame))
	s.Update(finish.Add(time.Minute))
	s.End(false)
	if calls != 1 {
		t.Fatalf("result calls %d", calls)
	}
}

func TestScotdDamageImmunity(t *testing.T) {
	now := time.Unix(100, 0)
	s := NewScotd(nil, 14605, 2)
	s.Rand = func(int) int { return 0 }
	s.Begin(now)
	s.enemies = []scotdEnemy{{x: s.X, y: s.Y, hp: 3, direction: 1}}
	s.Update(now.Add(arcadeFrame))
	if s.Lives != 2 {
		t.Fatalf("contact lives %d", s.Lives)
	}
	s.enemies[0].x = s.X
	s.Update(now.Add(2 * arcadeFrame))
	if s.Lives != 2 {
		t.Fatal("consecutive contact bypassed immunity")
	}
}

func TestPumpkinAimingAndResult(t *testing.T) {
	now := time.Unix(100, 0)
	p := NewPumpkin(nil)
	p.Rand = func(int) int { return 0 }
	p.Begin(now)
	p.Click(400, 340, now)
	if p.shot.active {
		t.Fatal("shot during countdown")
	}
	ready := now.Add(arcadeCountdown)
	p.ghosts[0] = pumpkinGhost{x: 400, y: 340, active: true}
	p.Click(400, 340, ready)
	p.Update(ready.Add(2 * arcadeFrame))
	if p.Score != 1 || !p.ghosts[0].hit || p.shot.active {
		t.Fatal("aimed hit did not settle")
	}
	p.Score = 30
	// Complete immediately before the deadline without accumulating further misses.
	p.frameAt = p.deadline.Add(-arcadeFrame)
	p.ghosts = [pumpkinGhostCount]pumpkinGhost{}
	calls := 0
	p.Result = func(win bool) {
		calls++
		if !win {
			t.Fatal("qualified score lost")
		}
	}
	p.Update(p.deadline)
	if calls != 0 {
		t.Fatal("result delay omitted")
	}
	p.Update(p.deadline.Add(pumpkinResultDelay))
	p.End(false)
	if calls != 1 {
		t.Fatalf("result calls %d", calls)
	}
}

func TestPumpkinMissAndFrameCatchup(t *testing.T) {
	now := time.Unix(100, 0)
	p := NewPumpkin(nil)
	p.Begin(now)
	p.ghosts[0] = pumpkinGhost{x: 55, y: 600, active: true, returning: true}
	p.Update(now.Add(arcadeCountdown + arcadeFrame))
	if p.Lives != 2 || p.ghosts[0].active {
		t.Fatal("miss did not cost exactly one life")
	}
	a, b := NewPumpkin(nil), NewPumpkin(nil)
	for _, g := range []*Pumpkin{a, b} {
		g.Rand = func(int) int { return 0 }
		g.Begin(now)
	}
	for i := 1; i <= 100; i++ {
		a.Update(now.Add(arcadeCountdown + time.Duration(i)*arcadeFrame))
	}
	b.Update(now.Add(arcadeCountdown + 100*arcadeFrame))
	if a.Score != b.Score || a.Lives != b.Lives || a.ghosts[0].y != b.ghosts[0].y {
		t.Fatal("catchup changes gameplay")
	}
}
