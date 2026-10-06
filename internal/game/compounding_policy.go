package game

// These are initial balancing choices, not claimed native probabilities.
// See docs/COMPOUNDING.md. Rebuild the server after editing this file.
const (
	AlchemyChanceScale                = 10000
	AlchemySkillMaximum               = 30 // Skill.dat field 105 for 15997..15999.
	AlchemyBaseLimit                  = 5
	AlchemyMinimumMaterials           = 2
	AlchemyMaximumIngredients         = 5
	AlchemyMinimumDelta               = -4
	AlchemyRankMinimum                = 1
	AlchemyPositiveLevelWeightPercent = 5
	AlchemyWeightPercentScale         = 100
	AlchemySkillEXPGain               = 1
	AlchemyPrimarySkill               = 15997
	AlchemyJuniorSkill                = 15998
	AlchemySuperiorSkill              = 15999
	AlchemyBookOne                    = 34009
	AlchemyBookTwo                    = 34010
	AlchemyBookThree                  = 34011
	AlchemyBookFour                   = 34012
	AlchemyCommonStone                = 43001
	AlchemyStrawMushroom              = 41005
	AlchemyDirtyWater                 = 60003 // WLRI names this item Sewage.
	AlchemyCrudeOil                   = 47001
	AlchemySteamedBuns                = 32024
)

type AlchemyTier byte

const (
	AlchemyPrimary AlchemyTier = iota
	AlchemyJunior
	AlchemySuperior
)

type alchemyCurve struct {
	Cap                                       int
	Catastrophe, CatastropheTrainingReduction int
	SecondaryDrop, SecondaryTrainingReduction int
	// Ordered from delta -4 through +4/+5.
	Weights []int
}

var alchemyCurves = [...]alchemyCurve{
	{4, 2400, 1000, 8000, 2000, []int{10, 18, 22, 25, 25, 18, 12, 7, 3}},
	{4, 1200, 600, 4500, 2000, []int{3, 6, 10, 12, 20, 25, 22, 16, 8}},
	{5, 400, 300, 0, 0, []int{1, 2, 4, 6, 15, 23, 26, 22, 16, 10}},
}

func AlchemyChances(tier AlchemyTier, level byte) (catastrophe, secondaryDrop int) {
	if tier > AlchemySuperior {
		return 0, 0
	}
	c := alchemyCurves[tier]
	n := int(min(level, AlchemySkillMaximum))
	return max(0, c.Catastrophe-c.CatastropheTrainingReduction*n/AlchemySkillMaximum), max(0, c.SecondaryDrop-c.SecondaryTrainingReduction*n/AlchemySkillMaximum)
}

// AlchemyDeltaWeights describes the unmodified roll; books shift its result.
func AlchemyDeltaWeights(tier AlchemyTier, level byte) []int {
	if tier > AlchemySuperior {
		return nil
	}
	c := alchemyCurves[tier]
	out := make([]int, c.Cap-AlchemyMinimumDelta+1)
	for i := range out {
		w := c.Weights[i]
		out[i] = w * AlchemyWeightPercentScale
		if i+AlchemyMinimumDelta > 0 {
			out[i] = w * (AlchemyWeightPercentScale + int(min(level, AlchemySkillMaximum))*AlchemyPositiveLevelWeightPercent)
		}
	}
	return out
}
func AlchemyBookBonus(id uint16) int {
	switch id {
	case AlchemyBookOne:
		return 1
	case AlchemyBookTwo:
		return 2
	case AlchemyBookThree:
		return 3
	case AlchemyBookFour:
		return 4
	}
	return 0
}
func (c Character) AlchemySkill() (tier AlchemyTier, level byte, id uint16) {
	for _, sk := range c.Skills {
		if sk.Grade == 0 {
			continue
		}
		var t AlchemyTier
		switch sk.ID {
		case AlchemyPrimarySkill:
			t = AlchemyPrimary
		case AlchemyJuniorSkill:
			t = AlchemyJunior
		case AlchemySuperiorSkill:
			t = AlchemySuperior
		default:
			continue
		}
		if id == 0 || t > tier || t == tier && sk.Grade > level {
			tier, level, id = t, min(sk.Grade, AlchemySkillMaximum), sk.ID
		}
	}
	return
}

// SkillGradeLimit preserves combat limits while allowing native alchemy grades.
func SkillGradeLimit(id uint16) byte {
	switch id {
	case AlchemyPrimarySkill, AlchemyJuniorSkill, AlchemySuperiorSkill:
		return AlchemySkillMaximum
	}
	return MaxSkillGrade
}
