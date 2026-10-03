package game

// PetGrowthFormula selects the weights for automatic level-up attribute points.
type PetGrowthFormula string

const (
	PetGrowthBaseStats   PetGrowthFormula = "base_stats"
	PetGrowthCombatStats PetGrowthFormula = "combat_stats"
)

func (f PetGrowthFormula) Valid() bool { return f == PetGrowthBaseStats || f == PetGrowthCombatStats }

// PetGrowthOptions supplies the current formula and catalog to each EXP grant.
// Combat weights map STR/CON/INT/WIS/AGI to ATK/DEF/MAT/MDF/SPD respectively.
// Vitals and temporary battle effects are excluded; the existing pet combat
// calculation includes level, permanent attributes, element and equipment.
type PetGrowthOptions struct {
	Formula PetGrowthFormula
	Items   map[uint16]ItemDefinition
}

func (p Pet) growthWeights(t PetTemplate, known bool, options []PetGrowthOptions) []int {
	if len(options) > 0 && options[0].Formula == PetGrowthCombatStats {
		full := p.Combat(options[0].Items)
		return []int{int(full.ATK), int(full.DEF), int(full.MAT), int(full.MDF), int(full.SPD)}
	}
	stats := p.Base
	if known {
		stats = t.Stats
	}
	return []int{int(stats.Strength), int(stats.Constitution), int(stats.Intelligence), int(stats.Wisdom), int(stats.Agility)}
}
