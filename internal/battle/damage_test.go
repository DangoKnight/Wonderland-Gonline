package battle

import (
	"math"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestAuthoredSkillPowerAndProvisionalGradeGrowth(t *testing.T) {
	const blastAttackSkill = 11166
	basePower := 20.0
	r := lowest
	r.Skills = map[uint16]assets.Skill{blastAttackSkill: {
		ID: blastAttackSkill, EffectLayer: assets.SkillLayerMagical,
		BasePower: &basePower, PowerPerLevel: 1.38, AdditionalDamage: 4, StatMultiplier: 1.1,
	}}
	for _, tc := range []struct {
		grade byte
		want  float64
	}{{1, 111}, {2, 112}, {10, 120}} {
		actor := &Fighter{Atk: 9999, Matk: 100, Char: &game.Character{Skills: []game.LearnedSkill{{ID: blastAttackSkill, Grade: tc.grade}}}}
		if got := r.damageBase(actor, &Fighter{Def: 9999, Mdef: 50}, blastAttackSkill); math.Abs(got-tc.want) > 1e-8 {
			t.Fatal("authored power must replace native coefficients, growing 5% per grade", tc, got)
		}
	}
	// An explicitly authored zero is different from an unresolved power.
	basePower = 0
	if got := r.damageBase(&Fighter{Matk: 100}, &Fighter{Mdef: 50}, blastAttackSkill); math.Abs(got-91) > 1e-8 {
		t.Fatal("explicit zero used native fallback", got)
	}
	s := r.Skills[blastAttackSkill]
	s.BasePower = nil
	r.Skills[blastAttackSkill] = s
	if got := r.damageBase(&Fighter{Matk: 100}, &Fighter{Mdef: 50}, blastAttackSkill); math.Abs(got-96.38) > 1e-8 {
		t.Fatal("unresolved compatibility grade-one estimate", got)
	}
}

func TestCommunityDamageBaseAndMagicalDefense(t *testing.T) {
	r := lowest
	r.Skills = map[uint16]assets.Skill{50001: {ID: 50001, EffectLayer: 2}, 50002: {ID: 50002, EffectLayer: 1, PowerPerLevel: 10, AdditionalDamage: 20}}
	for _, tt := range []struct {
		name                string
		atk, matk, def, mdf int
		skill               uint16
		want                float64
	}{
		{"physical above", 100, 0, 50, 0, 10001, 91},
		{"physical equal", 100, 0, 100, 0, 10001, 42},
		{"physical below", 100, 0, 200, 0, 10001, 24.1},
		{"magic ignores high physical defense", 0, 100, 9999, 50, 50001, 91},
		{"magic ignores low physical defense", 0, 100, 0, 50, 50001, 91},
		{"magic below MDEF keeps documented coefficient", 0, 100, 1, 200, 50001, -56},
		{"additive skill power", 100, 0, 50, 0, 50002, 121},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := &Fighter{Atk: tt.atk, Matk: tt.matk}, &Fighter{Def: tt.def, Mdef: tt.mdf}
			if got := r.damageBase(a, b, tt.skill); math.Abs(got-tt.want) > 1e-8 {
				t.Fatalf("base=%v want %v", got, tt.want)
			}
		})
	}
}

func TestCommunityElementAdjustmentsAndFinalVariance(t *testing.T) {
	r := lowest
	a, b := &Fighter{Atk: 100, Element: 3}, &Fighter{Def: 50, Element: 4}
	if got, _ := r.strikeDamage(Action{Actor: a, Kind: "attack", Skill: 10001}, b, false, 1); got != 146 {
		t.Fatal("Fire/Wind multiplier or late random roll", got)
	}
	if got, _ := r.strikeDamage(Action{Actor: a, Kind: "attack", Skill: 10001}, b, true, 2); got != 110 {
		t.Fatal("combo/defend should adjust elemental factor additively", got)
	}
	if got := clampDamage(13.999999999999998); got != 14 {
		t.Fatal("decimal coefficient rounding lost an integer point", got)
	}
}

func TestCommunityAreaCoefficients(t *testing.T) {
	for _, tt := range []struct {
		count int
		want  float64
	}{{1, 140}, {2, 85}, {3, 60}, {4, 60}, {6, 40}} {
		offsets := make([]assets.FormationOffset, tt.count)
		r := Rules{Skills: map[uint16]assets.Skill{50001: {ID: 50001, EffectLayer: 1, Targeting: []assets.SkillTargeting{{MinGrade: 1, MaxGrade: 10, Offsets: offsets}}}}}
		if got := r.damageBase(&Fighter{Atk: 100}, &Fighter{}, 50001); math.Abs(got-tt.want) > 1e-8 {
			t.Fatalf("%d targets: %v want %v", tt.count, got, tt.want)
		}
	}
}
