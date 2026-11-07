package battle

import (
	"math"
	"wonderland-gonline/internal/assets"
)

// Community model supplied in var/damage/formulas; see docs/DAMAGE.md for
// implementation choices where the experiments leave an exact rule unresolved.
const (
	damageSingleCoefficient       = 1.4
	damageBelowDefenseCoefficient = .599
	damageDefenseCoefficient      = .98
	damageHighDefenseCoefficient  = .179
	damageDefendElementAdjustment = -.7
	damageFreezeElementAdjustment = .1
	damageRoundingTolerance       = 1e-9 // Prevent decimal coefficients flooring an exact integer one point low.

	// Empirical whole-attack power, fitted to independent Legacy measurements.
	// Contacts divide this total during playback; they do not multiply it.
	damagePhysicalFixedPower    = 1.9131446584781497
	damagePhysicalGradePower    = .47524025502148415
	damageMagicalPower          = 2.0
	damageTwoTargetCoefficient  = .85
	damageMediumAreaCoefficient = .6
	damageLargeAreaCoefficient  = .4
	damageMediumAreaMinimum     = 3
	damageLargeAreaMinimum      = 6

	// Frozen quadratic correction selected from the Legacy damage investigation.
	// Polynomial inputs saturate at the measured range. Above either upper
	// bound, its contribution fades to zero: independent high-stat reference
	// examples favor the linear baseline, not a permanent extrapolated offset.
	damageMagicFitMinAttack             = 24
	damageMagicFitMaxAttack             = 1052
	damageMagicFitMinDefense            = 35
	damageMagicFitMaxDefense            = 78
	damageMagicAdjustmentFixed          = .244297946284
	damageMagicAdjustmentAttack         = .00607069954603
	damageMagicAdjustmentDefense        = .00653637105727
	damageMagicAdjustmentAttackSquared  = -.00000548960961612
	damageMagicAdjustmentCross          = .0000874824337356
	damageMagicAdjustmentDefenseSquared = .000170650039492
)

func clampDamage(value float64) int {
	return int(math.Max(1, math.Min(math.MaxInt32, math.Floor(value+damageRoundingTolerance))))
}

func (r Rules) damageBase(actor, target *Fighter, skill uint16) float64 {
	s, known := r.Skills[skill]
	magical := known && s.EffectLayer == assets.SkillLayerMagical || Basic(skill) && actor.Weapon == 5
	offense, defense := actor.attack(), target.defense()
	if magical {
		offense, defense = actor.magicAttack(), target.magicDefense()
	}
	coefficient, correction := damageSingleCoefficient, damageDefenseCoefficient
	grade := min(int(s.GradeLimit()), max(1, skillGrade(actor, skill)))
	power := 0.0
	if known && !Basic(skill) {
		power = damagePhysicalFixedPower*float64(s.AdditionalDamage) + damagePhysicalGradePower*s.PowerPerLevel*float64(grade)
		if magical {
			power = damageMagicalPower * (float64(s.AdditionalDamage) + s.PowerPerLevel*float64(grade))
		}
		count := damageTargetCapacity(s, grade)
		switch {
		case count >= damageLargeAreaMinimum:
			coefficient = damageLargeAreaCoefficient
		case count >= damageMediumAreaMinimum:
			coefficient = damageMediumAreaCoefficient
		case count == nativeColumnTargetLimit:
			coefficient = damageTwoTargetCoefficient
		}
		// A capacity-based approximation to the authored area powers. Never use
		// occupied cells or the numeric native pattern code as a power divisor.
		power *= 2 / float64(count+1)
	}
	// The low-ATK branch is established for single-target physical attacks.
	// Magic and area attacks retain their documented coefficients, rather than
	// extrapolating an unconfirmed low-offense branch to them.
	if !magical && coefficient == damageSingleCoefficient && offense < defense {
		coefficient, correction = damageBelowDefenseCoefficient, damageHighDefenseCoefficient
	}
	statDamage := coefficient*float64(offense) - correction*float64(defense)
	if magical && coefficient == damageSingleCoefficient {
		// The low-MAT Legacy samples favor a floor on the combined stat term,
		// before adding skill power. Otherwise estimated defense consumes P and
		// badly underpredicts weak-caster spells. This is an empirical rule;
		// area attacks retain their separate, less-tested stat calculation.
		statDamage = math.Max(0, statDamage+magicalDamageAdjustment(offense, defense))
	}
	return statDamage + power
}

// The fit changes only the stat term. P, elements, contacts and area coefficients
// retain their independently specified rules. See docs/DAMAGE.md for validation.
func magicalDamageAdjustment(offense, defense int) float64 {
	x := float64(min(damageMagicFitMaxAttack, max(damageMagicFitMinAttack, offense)))
	y := float64(min(damageMagicFitMaxDefense, max(damageMagicFitMinDefense, defense)))
	correction := damageMagicAdjustmentFixed + damageMagicAdjustmentAttack*x + damageMagicAdjustmentDefense*y +
		damageMagicAdjustmentAttackSquared*x*x + damageMagicAdjustmentCross*x*y + damageMagicAdjustmentDefenseSquared*y*y
	return correction * magicalDamageFitWeight(offense, damageMagicFitMaxAttack) * magicalDamageFitWeight(defense, damageMagicFitMaxDefense)
}

// Keep the measured interval unchanged, then smoothly return to the reference
// baseline by twice its upper bound. This is an extrapolation policy, not an
// inferred Legacy mechanic. Zero endpoint slopes avoid a stat-growth seam.
func magicalDamageFitWeight(value, upper int) float64 {
	if value <= upper {
		return 1
	}
	t := math.Min(1, (float64(value)-float64(upper))/float64(upper))
	return (1 - t) * (1 - t) * (1 + 2*t)
}

// Match the resolver's explicit-shape precedence and native compatibility shapes.
// Unknown patterns remain single-target, as they do in skillTargets.
func damageTargetCapacity(skill assets.Skill, grade int) int {
	if shape, ok := skill.TargetingAt(grade); ok {
		if shape.All {
			return assets.MaxBattleSideTargets
		}
		return max(1, len(shape.Offsets))
	}
	if skill.AreaAttack != nil && !*skill.AreaAttack {
		return 1
	}
	switch skill.NativePatternAt(grade) {
	case nativePatternColumn:
		return nativeColumnTargetLimit
	case nativePatternRow:
		return nativeRowTargetLimit
	case nativePatternCross:
		return nativeCrossTargetLimit
	case nativePatternFive:
		return nativeFiveTargetLimit
	case nativePatternSix:
		return nativeSixTargetLimit
	case nativePatternTeam, nativePatternAll:
		return assets.MaxBattleSideTargets
	default:
		return 1
	}
}

func (r Rules) damageElement(actor, target *Fighter, skill uint16, defending bool, participants int) float64 {
	element := actor.Element
	// Native element zero means neutral, not missing. In particular, neutral
	// skills such as Bloodsuck must not inherit Fire's damage bonus. Native code
	// 7 explicitly requests caster affinity, also used for basic attacks and
	// unavailable skill definitions.
	if s, ok := r.Skills[skill]; ok && !Basic(skill) && s.Element != assets.SkillElementCaster {
		element = s.Element
	}
	factor := Elemental(element, target.Element)
	if participants > 1 {
		factor += r.comboDamageMultiplier(participants) - 1
	}
	if defending {
		factor += damageDefendElementAdjustment
	}
	for _, effect := range target.Effects {
		if effect.Turns > 0 {
			switch r.Skills[effect.Source].StatusCode() {
			case assets.NativeStatusFreeze, assets.NativeStatusFreezeDebuff:
				return math.Max(0, factor+damageFreezeElementAdjustment)
			}
		}
	}
	return math.Max(0, factor)
}
