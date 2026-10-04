package battle

import (
	"fmt"
	"testing"
)

func TestParticipantComboDamageAnimationAndHPLoss(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for count := 1; count <= 8; count++ {
			t.Run(fmt.Sprintf("enabled=%t/count=%d", enabled, count), func(t *testing.T) {
				speeds := make([]int, count)
				b, r, actions := comboFixture(speeds...)
				r.ComboDamagePerParticipant = enabled
				multiplier := 1.0
				if count > 1 {
					multiplier = 1.3
					if enabled {
						multiplier = 1 + float64(count)/10
					}
				}
				expected := 0
				hits := make(map[byte]int)
				for _, a := range actions {
					damage := int(float64(r.baseDamage(a.Actor, b.Defenders[0], a.Skill, 1)) * multiplier)
					hits[a.Actor.Y] = damage
					expected += damage
				}
				steps, _ := r.attack(b, b.order(r, actions)[0], map[int]bool{})
				if len(steps) != 1 || len(steps[0].SkillUses) != count || b.Defenders[0].HP != 10000-expected {
					t.Fatal("combo HP/cost mismatch", b.Defenders[0].HP, expected, steps)
				}
				var animation []byte
				for _, p := range steps[0].Packets {
					if len(p) > 1 && p[0] == 50 && p[1] == 1 {
						animation = p
					}
				}
				if len(animation) != 2+count*19 {
					t.Fatalf("wrong animation size: %d", len(animation))
				}
				for i := 0; i < count; i++ {
					off := 2 + i*19
					// Each native hit record contains a little-endian damage at +14.
					got := int(animation[off+14]) | int(animation[off+15])<<8
					if got != hits[animation[off+3]] {
						t.Fatal("animation damage mismatch", got, hits)
					}
				}
			})
		}
	}
}

func TestParticipantComboDamageFailedChainAndUnavailableAttacker(t *testing.T) {
	t.Run("failed roll", func(t *testing.T) {
		b, r, actions := comboFixture(0, 0, 0)
		r.ComboDamagePerParticipant = true
		b.Defenders[0].Level = 30
		r.Float = func() float64 { return .99 }
		expected := 0
		for _, a := range actions {
			expected += r.baseDamage(a.Actor, b.Defenders[0], a.Skill, 1)
		}
		steps, _ := r.attack(b, b.order(r, actions)[0], map[int]bool{})
		if len(steps) != 3 || b.Defenders[0].HP != 10000-expected {
			t.Fatal("failed chain received damage bonus", steps, b.Defenders[0].HP)
		}
	})
	t.Run("unavailable fighter excluded", func(t *testing.T) {
		b, r, actions := comboFixture(0, 0, 0)
		r.ComboDamagePerParticipant = true
		group := b.order(r, actions)[0]
		actions[2].Actor.HP = 0
		expected := 0
		for _, a := range actions[:2] {
			expected += int(float64(r.baseDamage(a.Actor, b.Defenders[0], a.Skill, 1)) * 1.2)
		}
		steps, _ := r.attack(b, group, map[int]bool{})
		if len(steps) != 1 || len(steps[0].SkillUses) != 2 || b.Defenders[0].HP != 10000-expected {
			t.Fatal("unavailable fighter inflated bonus", steps, b.Defenders[0].HP)
		}
	})
}
