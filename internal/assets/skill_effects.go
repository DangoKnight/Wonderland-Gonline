package assets

import "fmt"

// EffectStat identifies a quantity modified by active buffs/debuffs.
type EffectStat string

const (
	EffectATK                 EffectStat = "atk"
	EffectMATK                EffectStat = "matk"
	EffectDEF                 EffectStat = "def"
	EffectMDEF                EffectStat = "mdef"
	EffectSPD                 EffectStat = "spd"
	EffectDamageDealt         EffectStat = "damage_dealt"
	EffectDamageTaken         EffectStat = "damage_taken"
	EffectPhysicalDamageTaken EffectStat = "physical_damage_taken"
	EffectMagicalDamageTaken  EffectStat = "magical_damage_taken"
)

type EffectTarget string

const (
	EffectAlly  EffectTarget = "ally"
	EffectEnemy EffectTarget = "enemy"
	EffectSelf  EffectTarget = "self"
)

// Percent is an additive change: -50 halves a value; +200 triples it.
type StatModifier struct {
	Stat    EffectStat `json:"stat"`
	Flat    int32      `json:"flat,omitempty"`
	Percent int32      `json:"percent,omitempty"`
}

// Empty StackGroup allows independent modifiers to add. Within a named group,
// only the strongest increase and reduction for each stat/operation contribute.
type SkillEffect struct {
	Target                  EffectTarget    `json:"target"`
	Rounds                  int             `json:"rounds"`
	StackGroup              string          `json:"stack_group,omitempty"`
	Modifiers               []StatModifier  `json:"modifiers,omitempty"`
	MissPhysicalAttacks     bool            `json:"miss_physical_attacks,omitempty"`
	MissAreaAttacks         bool            `json:"miss_area_attacks,omitempty"`
	BlockActions            bool            `json:"block_actions,omitempty"`
	BreakOnDamage           bool            `json:"break_on_damage,omitempty"`
	AllyAttackChancePercent int             `json:"ally_attack_chance_percent,omitempty"`
	PeriodicDamage          *PeriodicDamage `json:"periodic_damage,omitempty"`
	Presentation            string          `json:"presentation,omitempty"`
	OnHit                   bool            `json:"on_hit,omitempty"`
}

type PeriodicDamage struct {
	MaxHPPercent int32 `json:"max_hp_percent,omitempty"`
	Flat         int32 `json:"flat,omitempty"`
	Minimum      int32 `json:"minimum,omitempty"`
}

const (
	MaxSkillEffects            = 32
	MaxEffectModifiers         = 32
	MaxEffectRounds            = 255
	MaxEffectPercent           = 100
	MaxEffectPresentationBytes = 64
)

// ValidateSkillEffects rejects unsupported definitions before publishing a catalog.
func ValidateSkillEffects(effects []SkillEffect) error {
	if len(effects) > MaxSkillEffects {
		return fmt.Errorf("too many skill effects")
	}
	for i, e := range effects {
		if e.OnHit != effects[0].OnHit {
			return fmt.Errorf("effect %d: mixed cast/hit triggers are unsupported", i)
		}
		if e.Target != EffectAlly && e.Target != EffectEnemy && e.Target != EffectSelf {
			return fmt.Errorf("effect %d: unknown target %q", i, e.Target)
		}
		if e.Rounds < 1 || e.Rounds > MaxEffectRounds || len(e.Modifiers) > MaxEffectModifiers {
			return fmt.Errorf("effect %d: invalid duration/modifier count", i)
		}
		if len(e.Modifiers) == 0 && !e.MissPhysicalAttacks && !e.MissAreaAttacks && !e.BlockActions && e.AllyAttackChancePercent == 0 && e.PeriodicDamage == nil && e.Presentation == "" {
			return fmt.Errorf("effect %d: empty effect", i)
		}

		if e.AllyAttackChancePercent < 0 || e.AllyAttackChancePercent > MaxEffectPercent || len(e.Presentation) > MaxEffectPresentationBytes {
			return fmt.Errorf("effect %d: invalid behavior", i)
		}
		if p := e.PeriodicDamage; p != nil {
			if p.MaxHPPercent < 0 || p.MaxHPPercent > MaxEffectPercent || p.Flat < 0 || p.Minimum < 0 || p.MaxHPPercent == 0 && p.Flat == 0 && p.Minimum == 0 {
				return fmt.Errorf("effect %d: invalid periodic damage", i)
			}
		}
		for _, m := range e.Modifiers {
			switch m.Stat {
			case EffectATK, EffectMATK, EffectDEF, EffectMDEF, EffectSPD, EffectDamageDealt, EffectDamageTaken, EffectPhysicalDamageTaken, EffectMagicalDamageTaken:
			default:
				return fmt.Errorf("effect %d: unknown stat %q", i, m.Stat)
			}
		}
	}
	return nil
}
