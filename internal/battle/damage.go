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
	damageSkillPowerGradeIncrease = .05  // Provisional tuning: +5% of base power per grade above 1.
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
	grade := min(assets.MaxSkillGrade, max(1, skillGrade(actor, skill)))
	power := 0.0
	if known && !Basic(skill) {
		basePower := float64(s.AdditionalDamage) + s.PowerPerLevel // Compatibility fallback: native grade-one estimate, not verified power.
		if s.BasePower != nil {
			basePower = *s.BasePower
		}
		power = basePower * (1 + float64(grade-1)*damageSkillPowerGradeIncrease)
		if shape, ok := s.TargetingAt(grade); ok {
			count := len(shape.Offsets)
			if shape.All {
				count = assets.MaxBattleSideTargets
			}
			switch {
			case count >= 6:
				coefficient = .4
			case count >= 3:
				coefficient = .6
			case count == 2:
				coefficient = .85
			}
		}
	}
	// The low-ATK branch is established for single-target physical attacks.
	// Magic and area attacks retain their documented coefficients, rather than
	// extrapolating an unconfirmed low-offense branch to them.
	if !magical && coefficient == damageSingleCoefficient && offense < defense {
		coefficient, correction = damageBelowDefenseCoefficient, damageHighDefenseCoefficient
	}
	return coefficient*float64(offense) + power - correction*float64(defense)
}

func (r Rules) damageElement(actor, target *Fighter, skill uint16, defending bool, participants int) float64 {
	element := actor.Element
	if s, ok := r.Skills[skill]; ok && !Basic(skill) && s.Element != 0 {
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
