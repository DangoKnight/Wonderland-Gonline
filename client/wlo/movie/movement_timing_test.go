package movie

import (
	"math"
	"testing"
	"time"
)

func TestSlowActorMovementIndependentOfTickRate(t *testing.T) {
	for _, interval := range []time.Duration{time.Second / 30, time.Second / 120, time.Second / 240} {
		t.Run(interval.String(), func(t *testing.T) {
			now := time.Unix(0, 0)
			a := ActorState{TX: 100, TY: 50, Speed: speeds[2], at: now}
			end := now.Add(time.Second)
			for now.Before(end) {
				now = minTime(now.Add(interval), end)
				a.step(now)
			}
			if math.Abs(a.X-8) > 0.01 || math.Abs(a.Y-4) > 0.01 {
				t.Fatal("fractional movement lost between ticks", a.X, a.Y)
			}
		})
	}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
