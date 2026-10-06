package battle

import (
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestExecutedSkillReceipts(t *testing.T) {
	for _, skip := range []bool{false, true} {
		b, self := setup(1000)
		b.Submit(lowest, 7, 1, []byte{4, 2, 2, 2, 0xf9, 0x2a})
		if skip {
			self.ApplyEffect(50010, 0, assets.SkillEffect{Target: assets.EffectSelf, Rounds: 2, BlockActions: true})
		}
		steps, _ := b.Round(lowest)
		count := 0
		for _, step := range steps {
			for _, use := range step.SkillUses {
				if use.Actor != self || use.Skill != 11001 {
					t.Fatal(use)
				}
				count++
			}
		}
		want := 1
		if skip {
			want = 0
		}
		if count != want {
			t.Fatalf("skip %v: got %d receipts, want %d", skip, count, want)
		}
	}
}

func TestSupportSkillReceiptRequiresSP(t *testing.T) {
	for _, enough := range []bool{true, false} {
		b, self := setup(1000)
		rules := lowest
		rules.Skills = map[uint16]assets.Skill{11001: {ID: 11001, Name: "Heal", SP: 10}}
		if !enough {
			self.SP = 0
		}
		step := rules.support(b, Action{Actor: self, Skill: 11001, Kind: "heal", TX: self.X, TY: self.Y})
		want := 0
		if enough {
			want = 1
		}
		if len(step.SkillUses) != want {
			t.Fatal(enough, step.SkillUses)
		}
	}
}
