package movie

import (
	"testing"
	"time"
)

// play runs a movie at 60 frames a second, closing each line as soon as it
// shows, and returns the lines said and the time taken.
func play(t *testing.T, m *Movie) ([]Line, time.Duration) {
	t.Helper()
	now := time.Unix(0, 0)
	var said []Line
	p := &Player{M: m, Now: func() time.Time { return now }, Say: func(l Line) { said = append(said, l) },
		Talking: func() bool { return false }}
	p.Start()
	start := now
	for i := 0; i < 60*600 && !p.Ended(); i++ {
		now = now.Add(time.Second / 60)
		p.Tick()
	}
	if !p.Ended() {
		t.Fatalf("still at stage %d after ten minutes", p.Stage)
	}
	return said, now.Sub(start)
}

// TestBeachPlayback: the beach rescue says its twelve lines in stage order
// and ends.
func TestBeachPlayback(t *testing.T) {
	needAssets(t)
	m, err := Load(assets, 12008)
	if err != nil {
		t.Fatal(err)
	}
	said, took := play(t, m)
	if len(said) != len(m.Lines) {
		t.Fatalf("said %d of %d lines", len(said), len(m.Lines))
	}
	for i := 1; i < len(said); i++ {
		if said[i].Stage < said[i-1].Stage {
			t.Fatalf("lines out of order: %+v", said)
		}
	}
	t.Logf("12008 took %v", took)
}

// TestStormPlayback: the storm (123.sty) also runs to its end.
func TestStormPlayback(t *testing.T) {
	needAssets(t)
	m, err := Load(assets, 123)
	if err != nil {
		t.Fatal(err)
	}
	said, took := play(t, m)
	t.Logf("123 took %v with %d lines", took, len(said))
}

// TestActorStep: an actor moves at its speed class toward its target and
// arrives exactly.
func TestActorStep(t *testing.T) {
	now := time.Unix(0, 0)
	a := &ActorState{X: 0, Y: 0, TX: 80, TY: 40, Speed: speeds[3], at: now}
	now = now.Add(500 * time.Millisecond)
	if a.step(now) {
		t.Fatal("arrived too early")
	}
	if a.X != 40 || a.Y < 19.9 || a.Y > 20.1 {
		t.Fatalf("at %v,%v after 500 ms at 0.08 px/ms", a.X, a.Y)
	}
	now = now.Add(time.Second)
	if !a.step(now) || a.X != 80 || a.Y != 40 {
		t.Fatalf("at %v,%v, not arrived", a.X, a.Y)
	}
}
