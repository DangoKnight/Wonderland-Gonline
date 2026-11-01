package game

import "math"

const (
	rebornExpPower              = 3.3
	rebornExpExtraPower         = 4.9
	rebornExpExtraLevel         = 150
	rebornExpBaseBonus          = 50
	JobNone              byte   = 0
	JobKiller            byte   = 1
	JobWarrior           byte   = 2
	JobKnight            byte   = 3
	JobWit               byte   = 4
	JobPriest            byte   = 5
	JobSeer              byte   = 6
	RebornMinimumLevel          = 100
	rebornStatMultiplier        = 1.1
	RebornLevelDeduction        = 99
	TransferMirrorItemID        = 34018
	SkillMurder          uint16 = 15241
	SkillHolyShield      uint16 = 15240
	SkillKnightsSpirit   uint16 = 15242
	SkillCurse           uint16 = 15243
	SkillRest            uint16 = 15244
	SkillForetell        uint16 = 15245
)

func RebornJobSkill(job byte) uint16 {
	switch job {
	case JobKiller:
		return SkillMurder
	case JobWarrior:
		return SkillHolyShield
	case JobKnight:
		return SkillKnightsSpirit
	case JobWit:
		return SkillCurse
	case JobPriest:
		return SkillRest
	case JobSeer:
		return SkillForetell
	}
	return 0
}

func (c *Character) RebornAs(job byte, force bool) bool {
	if c.Reborn || c.Job != JobNone || job < JobKiller || job > JobSeer || (!force && c.Level < RebornMinimumLevel) {
		return false
	}
	level := byte(max(1, int(c.Level)-RebornLevelDeduction))
	c.Job, c.RebornJob, c.Reborn = job, job, true
	points := c.StatPoints
	c.SetLevel(level)
	c.StatPoints = points
	return true
}

func (c Character) RebornByte() byte {
	if c.Reborn {
		return 1
	}
	return 0
}

// The source multiplies rounded innate stats, before equipment bonuses.
func (c Character) classStats(atk, def, mat, mdf, spd int32) (int32, int32, int32, int32, int32) {
	if !c.Reborn {
		return atk, def, mat, mdf, spd
	}
	boost := func(v int32) int32 { return int32(uint16(math.RoundToEven(float64(v) * rebornStatMultiplier))) }
	switch c.Job {
	case JobKiller:
		atk, spd = boost(atk), boost(spd)
	case JobWarrior:
		atk, def = boost(atk), boost(def)
	case JobKnight:
		def, spd = boost(def), boost(spd)
	case JobWit:
		mat = boost(mat)
	case JobPriest:
		mdf = boost(mdf)
	case JobSeer:
		spd = boost(spd)
	}
	return atk, def, mat, mdf, spd
}

func (c Character) LevelRequirement(level int) uint64 {
	if !c.Reborn {
		return LevelExp(level)
	}
	need := uint64(math.Pow(float64(level+1), rebornExpPower))
	if level < rebornExpExtraLevel {
		return need + rebornExpBaseBonus
	}
	return need + uint64(math.Pow(float64(level+1-rebornExpExtraLevel), rebornExpExtraPower))
}
func (c Character) LevelFromEXP(total uint64) byte {
	level := 1
	for level < MaxLevel {
		need := c.LevelRequirement(level)
		if total < need {
			break
		}
		total -= need
		level++
	}
	return byte(level)
}
