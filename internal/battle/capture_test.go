package battle

import (
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestCaptureEligibilityBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		level                             int
		wounded, blocked, restricted, net bool
		want                              bool
	}{
		{"plus five", 6, true, false, false, false, true},
		{"plus six", 7, true, false, false, false, false},
		{"net plus ten", 11, true, false, false, true, true},
		{"net plus eleven", 12, true, false, false, true, false},
		{"full HP", 1, false, false, false, false, false},
		{"sealed", 1, true, true, false, false, false},
		{"content prohibits", 1, true, false, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, actor := setup(1000)
			target := b.Defenders[0]
			target.Level = byte(tc.level)
			if tc.wounded {
				target.HP--
			}
			if tc.blocked {
				target.ApplyEffect(12003, 0, assets.SkillEffect{Target: assets.EffectEnemy, Rounds: 2, BlockActions: true})
			}
			r := lowest
			r.NPCs = map[uint16]assets.NPC{uint16(target.Template): {}}
			if tc.restricted {
				r.NPCs[uint16(target.Template)] = assets.NPC{CaptureRestriction: 1}
			}
			if tc.net {
				r.PrepareCapture = func(Action) (int, bool) { return 10, true }
			}
			_, ok := r.capture(b, Action{Actor: actor, Player: actor.ID, TX: target.X, TY: target.Y})
			if !ok || (len(b.Captures) == 1) != tc.want {
				t.Fatal("capture eligibility", ok, b.Captures)
			}
		})
	}
}

func TestCaptureCommitFailureDoesNotRemoveTarget(t *testing.T) {
	b, actor := setup(1000)
	target := b.Defenders[0]
	target.HP--
	r := lowest
	r.PrepareCapture = func(Action) (int, bool) { return 0, false }
	steps, _ := r.capture(b, Action{Actor: actor, Player: actor.ID, TX: target.X, TY: target.Y})
	if target.Captured || len(b.Captures) != 0 || target.HP != 999 || len(steps) != 1 {
		t.Fatal("failed save captured target")
	}
}
