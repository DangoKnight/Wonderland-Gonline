package minigame

import (
	"testing"
	"time"
)

func TestMemoryOrderingAndDeadlines(t *testing.T) {
	now := time.Unix(100, 0)
	m := NewMemory(nil)
	m.Rand = func(int) int { return 0 }
	m.Begin(now)
	if len(m.cards) != 3 || m.cards[0].Value >= m.cards[1].Value {
		t.Fatal("initial sorted digits", m.cards)
	}
	first := m.cards[0]
	m.Click(first.X+1, first.Y+1, now)
	if m.next != 0 {
		t.Fatal("accepted click during countdown")
	}
	now = now.Add(arcadeCountdown)
	m.Update(now)
	now = now.Add(memoryPreviewTime)
	m.Update(now)
	wrong := m.cards[1]
	m.Click(wrong.X+1, wrong.Y+1, now)
	if m.Lives != 2 || m.phase != memoryWrong {
		t.Fatal("wrong digit did not cost one life")
	}
	m.Click(first.X+1, first.Y+1, now)
	if m.Lives != 2 {
		t.Fatal("duplicate mistake during feedback")
	}
	now = m.deadline
	m.Update(now)
	now = m.deadline
	m.Update(now)
	now = m.deadline
	m.Update(now)
	if m.phase != memoryPlaying {
		t.Fatal("new round did not hide digits")
	}
	now = m.deadline
	m.Click(m.cards[0].X+1, m.cards[0].Y+1, now)
	if m.Lives != 1 {
		t.Fatal("late click bypassed round timeout")
	}
}

func TestMemoryTwelveRoundsWinAndSingleResult(t *testing.T) {
	now := time.Unix(100, 0)
	m := NewMemory(nil)
	m.Rand = func(int) int { return 0 }
	wins, losses := 0, 0
	m.Result = func(win bool) {
		if win {
			wins++
		} else {
			losses++
		}
	}
	m.Begin(now)
	for round := 0; round < 12; round++ {
		now = m.deadline
		m.Update(now)
		now = m.deadline
		m.Update(now)
		if m.phase != memoryPlaying {
			t.Fatal("round not playable", round)
		}
		for _, card := range m.cards {
			m.Click(card.X+1, card.Y+1, now)
		}
		if m.Score != round+1 {
			t.Fatal("round completion", m.Score)
		}
		now = m.deadline
		m.Update(now)
	}
	if m.resultAt.IsZero() || wins != 0 {
		t.Fatal("missing result delay")
	}
	now = now.Add(arcadeResultWait)
	m.Update(now)
	m.Update(now.Add(time.Minute))
	m.End(false)
	if wins != 1 || losses != 0 || !m.Done() {
		t.Fatal("duplicate or incorrect result", wins, losses)
	}
}

func TestMemoryExhaustedLivesLose(t *testing.T) {
	now := time.Unix(100, 0)
	m := NewMemory(nil)
	m.Rand = func(int) int { return 0 }
	m.Begin(now)
	results := 0
	m.Result = func(win bool) {
		if win {
			t.Fatal("failed rounds won")
		}
		results++
	}
	for range 3 {
		now = m.deadline
		m.Update(now)
		now = m.deadline
		m.Update(now)
		now = m.deadline
		m.Update(now)
		now = m.deadline
		m.Update(now)
	}
	m.Update(now.Add(arcadeResultWait))
	if results != 1 || m.Lives != 0 {
		t.Fatal("exhaustion", results, m.Lives)
	}
}

func TestSheepDreamChoicesAndMisses(t *testing.T) {
	now := time.Unix(100, 0)
	g := NewSheep(nil, 1)
	g.Rand = func(int) int { return 1 }
	g.Begin(now)
	g.Click(350, 300, now) // Initial cue 0 is false.
	if g.Lives != 2 || g.Score != 0 {
		t.Fatal("false cue scored")
	}
	g.Click(350, 300, now)
	if g.Lives != 2 {
		t.Fatal("same cue accepted twice")
	}
	now = now.Add(750 * time.Millisecond)
	g.Update(now)
	g.Click(350, 300, now)
	if g.Lives != 2 || g.Score != 1 {
		t.Fatal("matching cue did not score")
	}
	now = now.Add(1500 * time.Millisecond)
	g.Update(now)
	if g.Lives != 1 {
		t.Fatal("unanswered matching cue did not cost a life", g.Lives)
	}
}

func TestSheepSurvivalAndGiveUp(t *testing.T) {
	now := time.Unix(100, 0)
	g := NewSheep(nil, 2)
	g.Rand = func(int) int { return 0 }
	results := 0
	won := false
	g.Result = func(win bool) { results++; won = win }
	g.Begin(now)
	for i := 1; i <= 30; i++ {
		g.Update(now.Add(time.Duration(i) * time.Second))
	}
	g.Update(now.Add(35 * time.Second))
	g.End(false)
	if results != 1 || !won {
		t.Fatal("survival", results, won)
	}
	g = NewSheep(nil, 1)
	results = 0
	g.Result = func(win bool) {
		results++
		if win {
			t.Fatal("Leave won")
		}
	}
	g.End(false)
	g.Begin(now)
	g.End(true)
	if results != 1 || g.Started {
		t.Fatal("leave before start lifecycle")
	}
}

func TestLuckyMovementAndCollision(t *testing.T) {
	now := time.Unix(100, 0)
	g := NewLucky(nil)
	g.Rand = func(int) int { return 14 }
	g.Begin(now)
	g.Aim(700, 0)
	g.Update(now.Add(arcadeCountdown - arcadeFrame))
	if g.x != 400 {
		t.Fatal("moved during countdown")
	}
	g.drops = []luckyDrop{{X: 410, Y: 500}, {X: 410, Y: 500, Bomb: true}, {X: 100, Y: 500}}
	g.Update(now.Add(arcadeCountdown + arcadeFrame))
	if g.x != 410 || g.Score != 1 || g.Lives != 2 || len(g.drops) != 1 {
		t.Fatal("collision/movement", g.x, g.Score, g.Lives, g.drops)
	}
	g.Aim(-100, 0)
	if g.aim != luckyLeft {
		t.Fatal("pointer bounds")
	}
}

func TestLuckyScoreAndBombResults(t *testing.T) {
	now := time.Unix(100, 0)
	for _, tc := range []struct {
		score, lives int
		win          bool
	}{{29, 3, false}, {30, 1, true}, {30, 0, false}} {
		g := NewLucky(nil)
		g.Rand = func(int) int { return 14 }
		g.Begin(now)
		g.Score, g.Lives = tc.score, tc.lives
		calls := 0
		g.Result = func(win bool) {
			calls++
			if win != tc.win {
				t.Fatal("wrong outcome", tc, win)
			}
		}
		g.Update(now.Add(arcadeCountdown + luckySeconds*time.Second))
		g.Update(now.Add(arcadeCountdown + luckySeconds*time.Second + arcadeResultWait))
		g.End(!tc.win)
		if calls != 1 {
			t.Fatal("duplicate result", calls)
		}
	}
}

func TestSheepDelayedFrameStillChargesMisses(t *testing.T) {
	now := time.Unix(100, 0)
	g := NewSheep(nil, 1)
	g.Rand = func(int) int { return 1 }
	g.Begin(now)
	g.Update(now.Add(30 * time.Second))
	if g.Lives != 0 || g.win {
		t.Fatal("delayed frame skipped missed dreams", g.Lives, g.win)
	}
}

func TestLuckyDelayedFrameStillCollides(t *testing.T) {
	now := time.Unix(100, 0)
	g := NewLucky(nil)
	g.Rand = func(int) int { return 14 }
	g.Begin(now)
	g.Score = 30
	g.drops = []luckyDrop{{X: 400, Y: 500, Bomb: true}, {X: 400, Y: 500, Bomb: true}, {X: 400, Y: 500, Bomb: true}}
	g.Update(now.Add(arcadeCountdown + 30*time.Second))
	if g.Lives != 0 || g.win {
		t.Fatal("delayed frame skipped bombs", g.Lives, g.win)
	}
}
