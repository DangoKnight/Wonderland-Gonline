package game

const (
	EquipmentBrokenDamage      = 100
	CombatWearRollScale        = 10000
	combatWearEqualLevelChance = 100  // 1% per equipped item per damaging contact.
	combatWearMaximumChance    = 2000 // 20%, reached at +20 enemy levels.
	combatWearMaximumLevelGap  = 20
)

func CombatWearChance(wearer, enemy byte) int {
	gap := int(enemy) - int(wearer)
	if gap < 0 {
		return 0
	}
	return combatWearEqualLevelChance + (combatWearMaximumChance-combatWearEqualLevelChance)*min(gap, combatWearMaximumLevelGap)/combatWearMaximumLevelGap
}

func (e *Equipment) ApplyCombatWear(items map[uint16]ItemDefinition, chance, contacts int, roll func(int, int) int) bool {
	changed := false
	if chance <= 0 {
		return false
	}
	for i, item := range e {
		d, known := items[item.ID]
		if item.Empty() || !known || d.EquipSlot == 0 || d.Indestructible || item.Damage >= EquipmentBrokenDamage {
			continue
		}
		for n := 0; n < contacts && e[i].Damage < EquipmentBrokenDamage; n++ {
			if roll(0, CombatWearRollScale) < chance {
				e[i].Damage++
				changed = true
			}
		}
	}
	return changed
}
