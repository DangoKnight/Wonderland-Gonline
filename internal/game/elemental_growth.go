package game

import (
	"fmt"
	"math"
)

// PointGrowth contains independent coefficients for each permanent attribute.
type PointGrowth struct {
	STR float64
	CON float64
	INT float64
	WIS float64
	AGI float64
}

func (g PointGrowth) value(a Attributes) float64 {
	return g.STR*float64(a.Strength) + g.CON*float64(a.Constitution) + g.INT*float64(a.Intelligence) + g.WIS*float64(a.Wisdom) + g.AGI*float64(a.Agility)
}

type StatGrowth struct {
	Level float64
	PointGrowth
}

func (g StatGrowth) value(level float64, a Attributes) float64 {
	return g.Level*level + g.PointGrowth.value(a)
}

// VitalGrowth keeps the fixed base and equipment outside the growth multiplier.
type VitalGrowth struct {
	StatGrowth
	Base       float64
	LevelPower float64
	Growth     PointGrowth
	Multiplier float64
}

func (g VitalGrowth) value(level float64, a Attributes) float64 {
	return g.Base + g.Multiplier*(g.StatGrowth.value(level, a)+math.Pow(level, g.LevelPower)*g.Growth.value(a))
}

type ElementGrowth struct {
	ATK StatGrowth
	DEF StatGrowth
	MAT StatGrowth
	MDF StatGrowth
	SPD StatGrowth
	HP  VitalGrowth
	SP  VitalGrowth
}
type ElementalGrowth struct {
	Earth ElementGrowth
	Water ElementGrowth
	Fire  ElementGrowth
	Wind  ElementGrowth
}

// baselineGrowth is the shared, pre-element formula used by native contribution packets.
func baselineGrowth() ElementGrowth {
	return ElementGrowth{
		ATK: StatGrowth{Level: 1.4, PointGrowth: PointGrowth{STR: 2}},
		DEF: StatGrowth{Level: 2, PointGrowth: PointGrowth{CON: 2}},
		MAT: StatGrowth{Level: 1.4, PointGrowth: PointGrowth{INT: 2}},
		MDF: StatGrowth{Level: 2, PointGrowth: PointGrowth{WIS: 2}},
		SPD: StatGrowth{Level: 1.6, PointGrowth: PointGrowth{AGI: 2.2}},
		HP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{CON: 2}}, Base: 180, LevelPower: .35, Growth: PointGrowth{CON: 2}, Multiplier: 1},
		SP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{WIS: 2}}, Base: 94, LevelPower: .3, Growth: PointGrowth{WIS: 3.2}, Multiplier: 1},
	}
}

// nativeElementGrowth is a fixed compatibility reference for aLogin contributions.
// Gameplay tuning belongs in growth_parameters.go; keep this reference unchanged.
func nativeElementGrowth(element byte) ElementGrowth {
	g := baselineGrowth()
	switch element {
	case Earth:
		g.DEF.Level = 3
	case Fire:
		g.ATK.Level = 2
		g.MAT.Level = 1.6
	case Wind:
		g.SPD.Level = 2.1
	}
	return g
}
func (g ElementalGrowth) ForElement(element byte) ElementGrowth {
	switch element {
	case Earth:
		return g.Earth
	case Water:
		return g.Water
	case Fire:
		return g.Fire
	case Wind:
		return g.Wind
	}
	return baselineGrowth()
}
func characterGrowth(element byte, configured []ElementalGrowth) ElementGrowth {
	g := DefaultElementalGrowth()
	if len(configured) > 0 && configured[0] != (ElementalGrowth{}) {
		g = configured[0]
	}
	return g.ForElement(element)
}

// Validate rejects nonfinite, negative and overflowing formulas in compiled tuning definitions.
func (g ElementalGrowth) Validate() error {
	const maxGrowthCoefficient = 1000
	const maxGrowthPower = 2
	check := func(name string, values ...float64) error {
		for _, v := range values {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > maxGrowthCoefficient {
				return fmt.Errorf("elemental growth %s coefficients must be finite and between 0 and %d", name, maxGrowthCoefficient)
			}
		}
		return nil
	}
	points := func(name string, p PointGrowth) error { return check(name, p.STR, p.CON, p.INT, p.WIS, p.AGI) }
	maxAttributes := Attributes{Strength: math.MaxUint16, Constitution: math.MaxUint16, Intelligence: math.MaxUint16, Wisdom: math.MaxUint16, Agility: math.MaxUint16}
	for name, e := range map[string]ElementGrowth{"earth": g.Earth, "water": g.Water, "fire": g.Fire, "wind": g.Wind} {
		for stat, s := range map[string]StatGrowth{"ATK": e.ATK, "DEF": e.DEF, "MAT": e.MAT, "MDF": e.MDF, "SPD": e.SPD, "HP": e.HP.StatGrowth, "SP": e.SP.StatGrowth} {
			if err := check(name+"."+stat, s.Level); err != nil {
				return err
			}
			if err := points(name+"."+stat, s.PointGrowth); err != nil {
				return err
			}
			if s.value(math.MaxUint8, maxAttributes) > math.MaxInt32 {
				return fmt.Errorf("elemental growth %s.%s exceeds combat limits", name, stat)
			}
		}
		for stat, v := range map[string]VitalGrowth{"HP": e.HP, "SP": e.SP} {
			if err := check(name+"."+stat, v.Base, v.Multiplier, v.LevelPower); err != nil {
				return err
			}
			if err := points(name+"."+stat+".growth", v.Growth); err != nil {
				return err
			}
			if v.LevelPower > maxGrowthPower || v.Multiplier == 0 || v.value(math.MaxUint8, maxAttributes) > math.MaxInt32 {
				return fmt.Errorf("elemental growth %s.%s has invalid power, multiplier or maximum", name, stat)
			}
		}
		if e.HP.value(1, Attributes{}) < 1 {
			return fmt.Errorf("elemental growth %s.HP must provide positive HP", name)
		}
	}
	return nil
}
