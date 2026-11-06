package game

const (
	CombatSkillEXPPerUse  uint32 = 10
	SupportSkillEXPPerUse uint32 = 15
	BuffSkillEXPBonus     uint32 = 25
	ReviveSkillEXPBonus   uint32 = 80
	SupportSkillEXPLimit  uint32 = 200
	skillEXPPercentScale         = 100
)

// CombatSkillEXP ignores overkill and rounds the damage percentage upward.
func CombatSkillEXP(damage, hpBefore, maxHP int) uint32 {
	if damage <= 0 || maxHP <= 0 {
		return CombatSkillEXPPerUse
	}
	effective := int64(damage)
	if hpBefore > 0 {
		effective = min(effective, int64(hpBefore))
	}
	percent := min(int64(skillEXPPercentScale), max(int64(0), (effective*skillEXPPercentScale+int64(maxHP)-1)/int64(maxHP)))
	return CombatSkillEXPPerUse + uint32(percent)
}

func HealingSkillEXP(restored, hpBefore, maxHP int) uint32 {
	if restored <= 0 || maxHP <= 0 {
		return SupportSkillEXPPerUse
	}
	effective := min(int64(restored), int64(max(0, maxHP-hpBefore)))
	percent := min(int64(skillEXPPercentScale), (effective*skillEXPPercentScale+int64(maxHP)-1)/int64(maxHP))
	return min(SupportSkillEXPLimit, SupportSkillEXPPerUse+uint32(percent*3/2))
}
