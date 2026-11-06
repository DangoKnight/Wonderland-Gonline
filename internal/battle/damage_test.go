package battle

import (
	"math"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestWholeAttackSkillPower(t *testing.T) {
	const blastAttackSkill = 11166
	basePower := 20.0
	r := lowest
	r.Skills = map[uint16]assets.Skill{blastAttackSkill: {
		ID: blastAttackSkill, EffectLayer: assets.SkillLayerMagical,
		BasePower: &basePower, PowerPerLevel: 1.38, AdditionalDamage: 4, StatMultiplier: 1.1, ContactEvents: 3,
	}}
	for _, tc := range []struct {
		grade byte
		want  float64
	}{{1, 103.7473276249973}, {2, 106.5073276249973}, {10, 128.5873276249973}} {
		actor := &Fighter{Atk: 9999, Matk: 100, Char: &game.Character{Skills: []game.LearnedSkill{{ID: blastAttackSkill, Grade: tc.grade}}}}
		if got := r.damageBase(actor, &Fighter{Def: 9999, Mdef: 50}, blastAttackSkill); math.Abs(got-tc.want) > 1e-8 {
			t.Fatal("whole-attack P plus bounded magical stat adjustment", tc, got)
		}
	}
	// Wiki powers remain reference data and do not replace the native formula.
	basePower = 0
	if got := r.damageBase(&Fighter{Matk: 100}, &Fighter{Mdef: 50}, blastAttackSkill); math.Abs(got-103.7473276249973) > 1e-8 {
		t.Fatal("reference power affected native formula", got)
	}
	s := r.Skills[blastAttackSkill]
	s.BasePower = nil
	s.ContactEvents = 0
	r.Skills[blastAttackSkill] = s
	if got := r.damageBase(&Fighter{Matk: 100}, &Fighter{Mdef: 50}, blastAttackSkill); math.Abs(got-103.7473276249973) > 1e-8 {
		t.Fatal("animation contacts must not change total damage", got)
	}
	s.StatMultiplier = 0
	r.Skills[blastAttackSkill] = s
	if got := r.damageBase(&Fighter{Matk: 100}, &Fighter{}, blastAttackSkill); math.Abs(got-152.30047960818255) > 1e-8 {
		t.Fatal("unresolved native multiplier must not scale MATK", got)
	}
}

func TestCommunityDamageBaseAndMagicalDefense(t *testing.T) {
	r := lowest
	r.Skills = map[uint16]assets.Skill{50001: {ID: 50001, EffectLayer: 2, StatMultiplier: 1}, 50002: {ID: 50002, EffectLayer: 1, PowerPerLevel: 10, AdditionalDamage: 20, StatMultiplier: 1}}
	for _, tt := range []struct {
		name                string
		atk, matk, def, mdf int
		skill               uint16
		want                float64
	}{
		{"physical above", 100, 0, 50, 0, 10001, 91},
		{"physical equal", 100, 0, 100, 0, 10001, 42},
		{"physical below", 100, 0, 200, 0, 10001, 24.1},
		{"magic ignores high physical defense", 0, 100, 9999, 50, 50001, 92.9873276249973},
		{"magic ignores low physical defense", 0, 100, 0, 50, 50001, 92.9873276249973},
		{"magic below MDEF floors stat contribution", 0, 100, 1, 200, 50001, 0},
		{"additive skill power", 100, 0, 50, 0, 50002, 134.01529571977784},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := &Fighter{Atk: tt.atk, Matk: tt.matk}, &Fighter{Def: tt.def, Mdef: tt.mdf}
			if got := r.damageBase(a, b, tt.skill); math.Abs(got-tt.want) > 1e-8 {
				t.Fatalf("base=%v want %v", got, tt.want)
			}
		})
	}
}

func TestDamageUsesPhysicalMagicStatsAndPetGrade(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		layer                byte
		atk, matk, def, mdef int
		want                 float64
	}{
		{"physical", 1, 100, 9999, 50, 9999, 112.03240760486743},
		{"physical below defense", 1, 100, 9999, 200, 9999, 45.13240760486743},
		{"magic", 2, 9999, 200, 9999, 50, 261.8671214597947},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := lowest
			r.Skills = map[uint16]assets.Skill{50000: {ID: 50000, EffectLayer: tc.layer, StatMultiplier: .5, AdditionalDamage: 10, PowerPerLevel: 2, ContactEvents: 3}}
			pet := &Fighter{Kind: Pet, Atk: tc.atk, Matk: tc.matk, Pet: &game.Pet{Skills: []game.PetSkill{{ID: 50000, Grade: 2}}}}
			if got := r.damageBase(pet, &Fighter{Def: tc.def, Mdef: tc.mdef}, 50000); math.Abs(got-tc.want) > 1e-8 {
				t.Fatal("whole-attack power must use the pet grade and relevant offense/defense", got, tc.want)
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

func TestDamageElementUsesNativeSkillElement(t *testing.T) {
	const (
		blastAttackSkill   = 11166
		starSkill          = 15071
		thunderAttackSkill = 11024
		bloodsuckSkill     = 15025
		turtlePalmSkill    = 25055
		scatterPowderSkill = 30065
		unavailableSkill   = 50000
	)
	r := Rules{Skills: map[uint16]assets.Skill{
		blastAttackSkill:   {ID: blastAttackSkill, Element: 3},
		starSkill:          {ID: starSkill, Element: 3},
		thunderAttackSkill: {ID: thunderAttackSkill, Element: 2},
		bloodsuckSkill:     {ID: bloodsuckSkill, Element: assets.SkillElementNeutral},
		turtlePalmSkill:    {ID: turtlePalmSkill, Element: assets.SkillElementCaster},
		scatterPowderSkill: {ID: scatterPowderSkill, Element: assets.SkillElementCaster},
		basicAttackSkill:   {ID: basicAttackSkill, Element: 2}, // Basic attacks retain actor affinity.
	}}
	for _, tc := range []struct {
		skill         uint16
		actor, target byte
		want          float64
	}{
		{blastAttackSkill, 1, 2, .7},
		{starSkill, 1, 3, 1.3},
		{thunderAttackSkill, 3, 3, 1.4},
		{bloodsuckSkill, 3, 4, 1},
		{bloodsuckSkill, 3, 2, 1},
		{bloodsuckSkill, 2, 1, .9},
		{turtlePalmSkill, 3, 4, 1.6},
		{turtlePalmSkill, 2, 1, .5},
		{scatterPowderSkill, 1, 2, 1.3},
		{scatterPowderSkill, 4, 1, 1.3},
		{unavailableSkill, 1, 2, 1.3},
		{basicAttackSkill, 3, 2, .7},
	} {
		if got := r.damageElement(&Fighter{Element: tc.actor}, &Fighter{Element: tc.target}, tc.skill, false, 1); got != tc.want {
			t.Fatalf("skill %d actor %d target %d: %v want %v", tc.skill, tc.actor, tc.target, got, tc.want)
		}
	}
}

func TestNeutralDamageRetainsEarthResistance(t *testing.T) {
	const neutralFixtureSkill = 50000
	r := lowest
	r.Skills = map[uint16]assets.Skill{neutralFixtureSkill: {
		ID: neutralFixtureSkill, Element: assets.SkillElementNeutral, EffectLayer: assets.SkillLayerPhysical,
	}}
	for _, caster := range []byte{0, game.Earth, game.Water, game.Fire, game.Wind} {
		action := Action{Actor: &Fighter{Atk: 100, Element: caster}, Kind: "attack", Skill: neutralFixtureSkill}
		for _, tc := range []struct {
			target byte
			want   int
		}{{game.Earth, 82}, {game.Water, 92}, {game.Fire, 92}, {game.Wind, 92}, {0, 92}} {
			got, _ := r.strikeDamage(action, &Fighter{Element: tc.target, Def: 50}, false, 1)
			if got != tc.want {
				t.Fatalf("caster %d neutral strike vs %d: got %d want %d", caster, tc.target, got, tc.want)
			}
		}
	}
}

func TestCommunityAreaCoefficients(t *testing.T) {
	for _, tt := range []struct {
		count int
		want  float64
	}{{1, 140}, {2, 85}, {3, 60}, {4, 60}, {5, 60}, {6, 40}, {8, 40}} {
		offsets := make([]assets.FormationOffset, tt.count)
		r := Rules{Skills: map[uint16]assets.Skill{50001: {ID: 50001, EffectLayer: 1, StatMultiplier: 1, Targeting: []assets.SkillTargeting{{MinGrade: 1, MaxGrade: 10, Offsets: offsets}}}}}
		if got := r.damageBase(&Fighter{Atk: 100}, &Fighter{}, 50001); math.Abs(got-tt.want) > 1e-8 {
			t.Fatalf("%d targets: %v want %v", tt.count, got, tt.want)
		}
	}
}

func TestNativeMaximumGradeControlsDamagePower(t *testing.T) {
	const skillID = 50000
	for _, tc := range []struct {
		maximum *byte
		want    float64
	}{{assets.GradeLimit(15), 60}, {assets.GradeLimit(5), 20}, {assets.GradeLimit(0), 0}, {nil, 40}} {
		r := Rules{Skills: map[uint16]assets.Skill{skillID: {ID: skillID, EffectLayer: assets.SkillLayerMagical, MaximumGrade: tc.maximum, PowerPerLevel: 2}}}
		for _, actor := range []*Fighter{
			{Char: &game.Character{Skills: []game.LearnedSkill{{ID: skillID, Grade: 20}}}},
			{Pet: &game.Pet{Skills: []game.PetSkill{{ID: skillID, Grade: 20}}}},
		} {
			// Subtract the independently evaluated zero-stat correction to isolate P.
			if got := r.damageBase(actor, &Fighter{}, skillID) - .8981372499698889; math.Abs(got-tc.want) > 1e-8 {
				t.Fatal("wrong grade cap in power", tc.maximum, got, tc.want)
			}
		}
	}
}

func TestDamageTargetCapacity(t *testing.T) {
	for _, tc := range []struct {
		pattern byte
		want    int
	}{{1, 1}, {nativePatternColumn, 2}, {nativePatternRow, 3}, {nativePatternCross, 4}, {nativePatternFive, 5}, {nativePatternSix, 6}, {nativePatternTeam, 8}, {nativePatternAll, 8}, {255, 1}} {
		skill := assets.Skill{NativePattern109: tc.pattern}
		if got := damageTargetCapacity(skill, 1); got != tc.want {
			t.Fatalf("pattern %d: capacity %d, want %d", tc.pattern, got, tc.want)
		}
		area := false
		skill.AreaAttack = &area
		if got := damageTargetCapacity(skill, 1); got != 1 {
			t.Fatal("explicit single-target override", got)
		}
		skill.Targeting = []assets.SkillTargeting{{MinGrade: 1, MaxGrade: 10, All: true}}
		if got := damageTargetCapacity(skill, 1); got != assets.MaxBattleSideTargets {
			t.Fatal("explicit shape precedence", got)
		}
	}
}

func TestNativeAreaDamageUsesGradeCapacity(t *testing.T) {
	const skillID = 50000
	r := Rules{Skills: map[uint16]assets.Skill{skillID: {
		ID: skillID, EffectLayer: assets.SkillLayerMagical, AdditionalDamage: 30,
		NativePattern109: nativePatternColumn, NativePattern110: nativePatternRow,
	}}}
	actor := &Fighter{Matk: 100, Char: &game.Character{Skills: []game.LearnedSkill{{ID: skillID, Grade: 1}}}}
	if got := r.damageBase(actor, &Fighter{}, skillID); math.Abs(got-125) > 1e-8 {
		t.Fatal("two-target capacity: .85*100 + 60*2/3", got)
	}
	actor.Char.Skills[0].Grade = 4
	if got := r.damageBase(actor, &Fighter{}, skillID); math.Abs(got-90) > 1e-8 {
		t.Fatal("three-target capacity: .6*100 + 60*2/4", got)
	}
}

// Independent Legacy observations, not expectations derived from the formula.
// These tolerances describe an approximation; low-MATK residuals remain larger.
func TestDamageApproximatesLegacyMeasurements(t *testing.T) {
	const skillID = 50000
	for _, tc := range []struct {
		name                                string
		layer, element, enemyElement, grade byte
		offense, defense                    int
		fixed                               uint16
		growth, observed, tolerance         float64
	}{
		{"Flame grade5 straw chicken", assets.SkillLayerPhysical, 3, 4, 5, 77, 34, 7, 2.8, 155.33, 3},
		{"Blast MAT25 straw chicken grade1", assets.SkillLayerMagical, 3, 4, 1, 25, 44, 4, 1.38, 22, 3},
		{"Blast MAT25 hyperkinetic chicken grade1", assets.SkillLayerMagical, 3, 3, 1, 25, 42, 4, 1.38, 17.5, 3},
		{"Blast MAT25 chicken grade2", assets.SkillLayerMagical, 3, 2, 2, 25, 39, 4, 1.38, 10.25, 2},
		{"Blast MAT29 chicken grade2", assets.SkillLayerMagical, 3, 2, 2, 29, 39, 4, 1.38, 14.75, 2},
		{"Blast MAT29 snail grade3", assets.SkillLayerMagical, 3, 1, 3, 29, 35, 4, 1.38, 33.67, 4},
		{"Blast high MAT female bug", assets.SkillLayerMagical, 3, 0, 4, 1052, 78, 4, 1.38, 1845, 20},
		{"Blast MAT238 male bug", assets.SkillLayerMagical, 3, 0, 1, 238, 74, 4, 1.38, 357, 5},
		{"Blast MAT238 female bug", assets.SkillLayerMagical, 3, 0, 1, 238, 78, 4, 1.38, 352.333, 5},
		{"Blast MAT238 hyperkinetic chicken", assets.SkillLayerMagical, 3, 3, 1, 238, 42, 4, 1.38, 404.5, 5},
		{"Blast MAT238 rice chicken", assets.SkillLayerMagical, 3, 1, 1, 238, 44, 4, 1.38, 373, 7},
		{"Blast MAT238 irrigated chicken", assets.SkillLayerMagical, 3, 2, 1, 238, 39, 4, 1.38, 217, 2},
		{"Blast MAT238 damp snail", assets.SkillLayerMagical, 3, 1, 1, 238, 35, 4, 1.38, 379.5, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := lowest
			r.Skills = map[uint16]assets.Skill{skillID: {ID: skillID, Element: tc.element, EffectLayer: tc.layer, AdditionalDamage: tc.fixed, PowerPerLevel: tc.growth}}
			actor := &Fighter{Atk: tc.offense, Matk: tc.offense, Element: tc.element, Char: &game.Character{Skills: []game.LearnedSkill{{ID: skillID, Grade: tc.grade}}}}
			target := &Fighter{Def: tc.defense, Mdef: tc.defense, Element: tc.enemyElement}
			minimum, _ := r.strikeDamage(Action{Actor: actor, Kind: "attack", Skill: skillID}, target, false, 1)
			midpoint := float64(minimum + 2) // Independent bounded variance is 1..5.
			if math.Abs(midpoint-tc.observed) > tc.tolerance {
				t.Fatalf("predicted midpoint %.2f, Legacy mean %.2f (tolerance %.2f)", midpoint, tc.observed, tc.tolerance)
			}
		})
	}
}

func TestMagicalDamageCorrectionFadesAndPreservesMonotonicity(t *testing.T) {
	r := Rules{Skills: map[uint16]assets.Skill{50000: {ID: 50000, EffectLayer: assets.SkillLayerMagical}}}
	base := func(offense, defense int) float64 {
		return r.damageBase(&Fighter{Matk: offense}, &Fighter{Mdef: defense}, 50000)
	}
	for _, offense := range []int{0, 1, 23, 24, 25, 77, 78, 79, 238, 1051, 1052, 1053, 1578, 2103, 2104, 2105, 5000, 100000} {
		for _, defense := range []int{0, 1, 34, 35, 36, 77, 78, 79, 117, 155, 156, 157, 5000, 100000} {
			value := base(offense, defense)
			if base(offense+1, defense) < value || base(offense, defense+1) > value {
				t.Fatalf("non-monotonic magic: MAT %d MDEF %d", offense, defense)
			}
		}
	}
	if got := magicalDamageAdjustment(100000, 100000); got != 0 {
		t.Fatal("correction must fade outside measured stats", got)
	}
	for _, tc := range []struct{ offense, defense int }{{2104, 50}, {238, 156}} {
		if got := magicalDamageAdjustment(tc.offense, tc.defense); got != 0 {
			t.Fatal("either unmeasured high stat must restore the baseline", tc, got)
		}
	}
	if got, want := magicalDamageAdjustment(1578, 50), magicalDamageAdjustment(1052, 50)/2; math.Abs(got-want) > 1e-8 {
		t.Fatal("attack fade midpoint", got, want)
	}
	if got, want := magicalDamageAdjustment(238, 117), magicalDamageAdjustment(238, 78)/2; math.Abs(got-want) > 1e-8 {
		t.Fatal("defense fade midpoint", got, want)
	}
	if got, want := magicalDamageAdjustment(0, 0), magicalDamageAdjustment(24, 35); got != want {
		t.Fatal("correction extrapolated beyond measured lower bounds", got, want)
	}
	if got := base(5001, 5000) - base(5000, 5000); math.Abs(got-1.4) > 1e-8 {
		t.Fatal("unmeasured high MAT must retain linear baseline growth", got)
	}
	if got := base(5000, 5001) - base(5000, 5000); math.Abs(got+.98) > 1e-8 {
		t.Fatal("unmeasured high MDEF must retain linear baseline reduction", got)
	}
}

// The saved 2022 single-target article gives these independent measured means
// with MDEF > DEF, so its max(DEF,MDEF) policy agrees with our MDEF-only rule.
// Reference P is held explicitly fixed here to test the stat term, not our
// separate native-P estimate. None of these observations trained the fit.
func TestHighStatCommunityMeasurementsRetainLinearBaseline(t *testing.T) {
	const referencePowerSkill = 50000
	for _, tc := range []struct {
		name       string
		matk, mdef int
		element    byte
		fixed      uint16
		observed   float64
	}{
		{"article row14", 2346, 360, 2, 220, 3036.5},
		{"article row15", 2346, 561, 2, 220, 2860.25},
		{"article row17", 1429, 561, 1, 25, 903},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := lowest
			r.Skills = map[uint16]assets.Skill{referencePowerSkill: {
				ID: referencePowerSkill, Element: tc.element,
				EffectLayer: assets.SkillLayerMagical, AdditionalDamage: tc.fixed,
			}}
			a := Action{Actor: &Fighter{Matk: tc.matk}, Kind: "attack", Skill: referencePowerSkill}
			minimum, _ := r.strikeDamage(a, &Fighter{Element: 4, Mdef: tc.mdef}, false, 1)
			if tc.observed < float64(minimum) || tc.observed > float64(minimum+4) {
				t.Fatalf("Legacy mean %.2f outside predicted %d..%d", tc.observed, minimum, minimum+4)
			}
		})
	}
}
