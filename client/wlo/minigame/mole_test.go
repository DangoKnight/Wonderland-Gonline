package minigame

import (
	"testing"
	"time"

	"wonderland-gonline/client/wlo/picdb"
)

func runningMole(t *testing.T) (*Mole, *time.Time, *[]string) {
	t.Helper()
	now := time.Unix(0, 0)
	m := NewMole(picdb.New(), MouseParam, now)
	var sounds []string
	m.Sound = func(p string) { sounds = append(sounds, p) }
	m.Rand = func(n int) int { return 0 }
	m.Start()
	// 3, 2, 1 beep, "Go", then a second later the game runs.
	for range 6 {
		now = now.Add(moleSecond)
		m.Update(now)
	}
	if m.state != moleStateRunning {
		t.Fatalf("state %d after the countdown", m.state)
	}
	return m, &now, &sounds
}

// TestMoleCountdown: three beeps, then "Go".
func TestMoleCountdown(t *testing.T) {
	_, _, sounds := runningMole(t)
	want := []string{soundBeep, soundBeep, soundBeep, soundGo}
	if len(*sounds) != len(want) {
		t.Fatalf("sounds %q", *sounds)
	}
	for i := range want {
		if (*sounds)[i] != want[i] {
			t.Fatalf("sounds %q", *sounds)
		}
	}
}

// TestMoleHits: a risen mole scores one; a bomb costs three, not below 0.
func TestMoleHits(t *testing.T) {
	m, now, _ := runningMole(t)
	h := &m.holes[1]
	h.state, h.kind, h.frame = holeUp, moleKindMole, moleFrames
	m.Click(h.x+0x30, h.y+0x10, *now)
	if m.score != 1 || h.state != holeHit {
		t.Fatalf("score %d state %d", m.score, h.state)
	}
	if m.hammer != hammerDown {
		t.Fatal("hammer did not swing")
	}
	*now = now.Add(2 * moleHammerHit)
	m.Update(*now)
	*now = now.Add(2 * moleHammerHit)
	m.Update(*now)
	b := &m.holes[2]
	b.state, b.kind, b.frame = holeUp, moleKindBomb, moleFrames
	m.Click(b.x+0x30, b.y+0x10, *now)
	if m.score != 0 || b.state != holeEmpty {
		t.Fatalf("after the bomb score %d state %d", m.score, b.state)
	}
}

// TestMoleResult: three seconds after the clock runs out the game reports
// a win for 30 points or more.
func TestMoleResult(t *testing.T) {
	for _, tc := range []struct {
		score int
		win   bool
	}{{29, false}, {30, true}} {
		m, now, _ := runningMole(t)
		var got []bool
		m.Result = func(win bool) { got = append(got, win) }
		m.score = tc.score
		for range moleSeconds {
			*now = now.Add(moleSecond)
			m.Update(*now)
			m.score = tc.score
		}
		if m.state != moleStateOver || len(got) != 0 {
			t.Fatalf("state %d results %v when the clock ran out", m.state, got)
		}
		*now = now.Add(moleEndWait)
		m.Update(*now)
		if len(got) != 1 || got[0] != tc.win || !m.Done() {
			t.Fatalf("score %d: results %v", tc.score, got)
		}
	}
}
