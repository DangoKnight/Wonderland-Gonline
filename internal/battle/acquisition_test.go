package battle

import "testing"

func TestKillingComboRewardsAfterItsAnimation(t *testing.T) {
	b, r, actions := comboFixture(100, 100, 100)
	b.Defenders[0].HP, b.Defenders[0].MaxHP = 1, 1000
	calls := 0
	r.RewardKill = func(target *Fighter, contributors []Action) Step {
		calls++
		if !target.Dead() || len(contributors) != 3 {
			t.Fatal("wrong killing combo", target.HP, len(contributors))
		}
		return Step{OwnerPackets: map[uint32][][]byte{1: {{23, 6}}}}
	}
	steps, _ := r.attack(b, actions, nil)
	if calls != 1 || len(steps) < 2 || steps[0].Delay == 0 || len(steps[1].OwnerPackets[1]) != 1 {
		t.Fatal("rewards not sequenced after contact", calls, steps)
	}
}
