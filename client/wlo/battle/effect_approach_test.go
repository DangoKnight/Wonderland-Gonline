package battle

import (
	"image"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/login"
)

func TestFirstFrameCueWaitsForApproach(t *testing.T) {
	m := Motion{Frames: []MotionFrame{{Offset: image.Pt(300, 0), Hold: 100 * time.Millisecond}, {Offset: image.Pt(360, 0), Speed: .6}}}
	if got := m.EventAt(MotionEvent{Trigger: 1}); got != 500*time.Millisecond {
		t.Fatal("first frame fired before arrival", got)
	}
	if got := m.EventAt(MotionEvent{Trigger: 2}); got != 630*time.Millisecond {
		t.Fatal("later frame trigger changed", got)
	}
	for _, trigger := range []uint32{0, 150} {
		if got := m.EventAt(MotionEvent{Trigger: trigger}); got != time.Duration(trigger)*time.Millisecond {
			t.Fatal("elapsed-time cue changed", trigger, got)
		}
	}
	m.Frames[0].Offset = image.Point{}
	if got := m.EventAt(MotionEvent{Trigger: 1}); got != 0 {
		t.Fatal("stationary cast was delayed", got)
	}
}

func TestNativeBeeEffectsWaitForApproach(t *testing.T) {
	a, err := Load(login.NewAssets("../../../data"))
	if err != nil {
		t.Fatal(err)
	}
	// Npc.dat: Honey/Worker Bee and Baby Bee respectively.
	for _, skill := range []uint16{30027, 30033} {
		m, ok := a.Motions[skill]
		if !ok || len(m.Frames) == 0 {
			t.Fatal("missing bee motion", skill)
		}
		arrival := m.frameSpans()[0] - m.Frames[0].Hold - motionTick
		if arrival <= 0 {
			t.Fatal("bee fixture has no approach", skill)
		}
		found := false
		for _, effect := range m.Effects {
			if effect.Trigger != 1 {
				continue
			}
			found = true
			one := m
			one.Effects = []MotionEffect{effect}
			for _, actor := range []Cell{{2, 2}, {4, 2}} {
				target := Cell{4, 2}
				if actor.X == 4 {
					target.X = 2
				}
				if got := one.EffectsAt(actor, target, arrival-time.Nanosecond); len(got) != 0 {
					t.Fatal("effect appeared during approach", skill, got)
				}
				if got := one.EffectsAt(actor, target, arrival); len(got) != 1 {
					t.Fatal("effect missing on arrival", skill, got)
				}
			}
		}
		if !found {
			t.Fatal("bee fixture has no first-frame effect", skill)
		}
	}
}
