package battle

import (
	"math"
	"testing"
	"wonderland-go/internal/game"
)

func TestComboChanceLinearLevelGap(t *testing.T) {
	for _, tc := range []struct {
		enemy byte
		want  float64
	}{
		{1, 1}, {5, 1}, {10, .9}, {20, .7}, {30, .5}, {40, .3}, {50, .1}, {55, 0}, {80, 0},
	} {
		b, _, _ := comboFixture(150, 60, 30)
		b.Defenders[0].Level = tc.enemy
		if got := b.comboChance(Attacker, b.Defenders[0]); math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("enemy %d: %.6f, want %.6f", tc.enemy, got, tc.want)
		}
	}
}

func TestComboChanceUsesWholePresentPlayerPetRoster(t *testing.T) {
	b, _, actions := comboFixture(150, 60, 30, 20, 10)
	// The final fighter is a pet. Non-attacking and knocked-out members still
	// belong to the present roster, so the mean is 23.2, not just attacker level.
	for i, level := range []byte{40, 40, 10, 10, 16} {
		actions[i].Actor.Level = level
	}
	actions[2].Kind = "effect"
	actions[3].Actor.HP = 0
	target := b.Defenders[0]
	target.Level = 20
	if got := b.comboChance(Attacker, target); math.Abs(got-.564) > 1e-12 {
		t.Fatal("whole roster mean", got)
	}
	actions[4].Actor.Absent = true
	if got := b.comboChance(Attacker, target); got != .6 {
		t.Fatal("absent pet affected mean", got)
	}
	for _, f := range b.Attackers {
		f.Absent = true
	}
	if b.comboChance(Attacker, target) != 0 {
		t.Fatal("empty party chance")
	}
	if b.comboChance(Attacker, nil) != 0 {
		t.Fatal("missing target chance")
	}
}

func TestComboChanceRollsOnceAndFailedChainsAttackSeparately(t *testing.T) {
	for _, tc := range []struct {
		roll     float64
		combined bool
	}{{.499, true}, {.5, false}, {.9, false}} {
		b, r, actions := comboFixture(150, 60, 30)
		b.Defenders[0].Level = 30
		calls := 0
		r.Float = func() float64 { calls++; return tc.roll }
		steps, _ := r.attack(b, b.order(r, actions)[0], map[int]bool{})
		if calls != 1 {
			t.Fatal("chain rerolled", calls)
		}
		damage := 0
		for _, step := range steps {
			if tc.combined {
				if len(step.SkillUses) != 3 {
					t.Fatal("successful chain split")
				}
			} else if len(step.SkillUses) != 1 {
				t.Fatal("failed chain retained combo bonus")
			}
			for _, a := range step.SkillUses {
				hit := r.baseDamage(a.Actor, b.Defenders[0], a.Skill, 1)
				if tc.combined {
					hit = int(float64(hit) * 1.3)
				}
				damage += hit
			}
		}
		if b.Defenders[0].HP != 10000-damage {
			t.Fatal("incorrect combo bonus", b.Defenders[0].HP, damage)
		}
	}
}

func TestComboChanceEndpointsAndSinglesDoNotRoll(t *testing.T) {
	for _, enemy := range []byte{5, 55} {
		b, r, actions := comboFixture(150, 60, 30)
		b.Defenders[0].Level = enemy
		r.Float = func() float64 { t.Fatal("endpoint consumed randomness"); return 0 }
		steps, _ := r.attack(b, b.order(r, actions)[0], map[int]bool{})
		if enemy == 5 && (len(steps) != 1 || len(steps[0].SkillUses) != 3) {
			t.Fatal("guaranteed chain failed")
		}
		if enemy == 55 && len(steps) != 3 {
			t.Fatal("zero chance joined chain")
		}
		r.attack(b, actions[:1], map[int]bool{})
	}
}

func TestComboRebirthEffectiveLevels(t *testing.T) {
	for _, tc := range []struct {
		level  byte
		reborn bool
		want   int
	}{{1, false, 1}, {1, true, 100}, {156, true, 255}, {199, true, 298}, {255, true, 354}} {
		for _, pet := range []bool{false, true} {
			f := &Fighter{Level: tc.level}
			if pet {
				f.Pet = &game.Pet{Reborn: tc.reborn}
			} else {
				f.Char = &game.Character{Reborn: tc.reborn}
			}
			if got := f.comboLevel(); got != tc.want {
				t.Fatalf("pet=%v level=%d reborn=%v: %d", pet, tc.level, tc.reborn, got)
			}
			if f.Level != tc.level {
				t.Fatal("rebirth changed visible level")
			}
		}
	}
}

func TestComboRebirthBonusBeforePartyAverage(t *testing.T) {
	b, _, actions := comboFixture(150, 60, 30, 20, 10)
	for _, a := range actions {
		a.Actor.Level = 1
	}
	actions[0].Actor.Char.Reborn = true
	actions[4].Actor.Pet.Reborn = true
	target := b.Defenders[0]
	target.Level = 40
	// (100+1+1+1+100)/5 = 40.6; gap +0.6 gives 51.2 percent.
	if got := b.comboChance(Attacker, target); math.Abs(got-.512) > 1e-12 {
		t.Fatal("rebirth averaged incorrectly", got)
	}
	actions[4].Actor.Absent = true
	// (100+1+1+1)/4 = 25.75; gap -14.25 gives 21.5 percent.
	if got := b.comboChance(Attacker, target); math.Abs(got-.215) > 1e-12 {
		t.Fatal("absent reborn pet contributed", got)
	}
}

func TestComboRebirthTargetUsesEffectiveLevel(t *testing.T) {
	b, _, actions := comboFixture(150, 60)
	for _, a := range actions {
		a.Actor.Level = 100
	}
	target := &Fighter{Level: 1, Pet: &game.Pet{Reborn: true}}
	if b.comboChance(Attacker, target) != .5 {
		t.Fatal("reborn target level ignored")
	}
}
