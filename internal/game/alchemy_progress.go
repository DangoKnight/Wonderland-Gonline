package game

import "math"

// Formula.dat coefficient 30 and level_scaler; FUN_0036e96c/0036e9b8.
const (
	alchemyExperienceExponent = 3.1
	alchemyExperienceOffset   = 5
)

func IsAlchemySkill(id uint16) bool {
	return id == AlchemyPrimarySkill || id == AlchemyJuniorSkill || id == AlchemySuperiorSkill
}

// AlchemyGradeEXP is the native EXP required to advance from grade to grade+1.
func AlchemyGradeEXP(grade byte) uint32 {
	return uint32(math.RoundToEven(math.Pow(float64(grade)+1, alchemyExperienceExponent))) + alchemyExperienceOffset
}

// AlchemyCumulativeEXP projects per-grade SQL EXP into the native skill counter.
func AlchemyCumulativeEXP(grade byte, exp uint32) uint32 {
	total := uint64(exp)
	for g := byte(1); g < grade; g++ {
		total += uint64(AlchemyGradeEXP(g))
	}
	return uint32(min(total, uint64(^uint32(0))))
}

func AlchemyGradeProgress(grade byte, cumulative uint32) uint32 {
	base := AlchemyCumulativeEXP(grade, 0)
	if cumulative < base {
		return 0
	}
	return cumulative - base
}
