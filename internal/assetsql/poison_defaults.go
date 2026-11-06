package assetsql

import (
	"gorm.io/gorm"
	"reflect"
	"strings"
	"wonderland-gonline/internal/assets"
)

const (
	nativePoisonLayer     = 15
	nativePoisonCode      = 171
	previousPoisonPercent = 8
	previousPoisonMinimum = 5
	nativePoisonPercent   = 10
	nativePoisonMinimum   = 15
)

func upgradePoisonEffect(e *assets.SkillEffect) bool {
	old := assets.SkillEffect{Target: assets.EffectEnemy, Rounds: e.Rounds, StackGroup: "periodic_poison", PeriodicDamage: &assets.PeriodicDamage{MaxHPPercent: previousPoisonPercent, Minimum: previousPoisonMinimum}}
	if !reflect.DeepEqual(*e, old) {
		return false
	}
	damage := *e.PeriodicDamage
	damage.MaxHPPercent, damage.Minimum = nativePoisonPercent, nativePoisonMinimum
	e.PeriodicDamage = &damage
	return true
}
func seedPoisonUpgrade(c *assets.Catalog) {
	for id, definition := range c.SkillEffects {
		if !strings.HasPrefix(id, "native_15_171_rounds_") {
			continue
		}
		for i := range definition.Effects {
			upgradePoisonEffect(&definition.Effects[i])
		}
		c.SkillEffects[id] = definition
	}
	for id, skill := range c.Skills {
		if skill.EffectLayer != nativePoisonLayer || skill.NativeEffectCode52 != nativePoisonCode {
			continue
		}
		for i := range skill.Effects {
			upgradePoisonEffect(&skill.Effects[i])
		}
		c.Skills[id] = skill
	}
}

// Change only an exact previously shipped poison effect. Customized effects,
// including different damage, modifiers, presentation and stacking, remain intact.
func migratePoisonDefaults(tx *gorm.DB) error {
	definitions, err := readSkillEffects(tx)
	if err != nil {
		return err
	}
	update := map[string]any{"value_periodic_damage_max_hp_percent": nativePoisonPercent, "value_periodic_damage_minimum": nativePoisonMinimum}
	for id, definition := range definitions {
		if !strings.HasPrefix(id, "native_15_171_rounds_") {
			continue
		}
		for i, e := range definition.Effects {
			if !upgradePoisonEffect(&e) {
				continue
			}
			if err = tx.Model(&SkillEffectsEffectsRow{}).Where("skill_effects_key = ? AND skill_effects_effects_ordinal = ?", id, i).Updates(update).Error; err != nil {
				return err
			}
		}
	}
	var skills []SkillsRow
	if err = tx.Where(&SkillsRow{ValueEffectLayer: nativePoisonLayer, ValueNativeEffectCode52: nativePoisonCode}).Find(&skills).Error; err != nil {
		return err
	}
	for _, skill := range skills {
		effects, err := readSkillsEffects(tx, skill.SkillsKey)
		if err != nil {
			return err
		}
		for i, e := range effects {
			if !upgradePoisonEffect(&e) {
				continue
			}
			if err = tx.Model(&SkillsEffectsRow{}).Where("skills_key = ? AND skills_effects_ordinal = ?", skill.SkillsKey, i).Updates(update).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
