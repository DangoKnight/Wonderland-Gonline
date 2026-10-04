package game

// DefaultElementalGrowth returns the compiled player growth parameters.
// Edit these coefficients and rebuild the server to change gameplay.
// Each call returns an independent value; no runtime configuration is used.
// Combat coefficients follow docs/References (WLRI Japanese wiki, standard
// elemental stat table); HP/SP coefficients are decoded from Formula.Dat.
func DefaultElementalGrowth() ElementalGrowth {
	return ElementalGrowth{
		Earth: ElementGrowth{
			ATK: StatGrowth{Level: 1.4, PointGrowth: PointGrowth{STR: 2}},
			DEF: StatGrowth{Level: 2.6, PointGrowth: PointGrowth{CON: 1.75}},
			MAT: StatGrowth{Level: 1.4, PointGrowth: PointGrowth{INT: 2}},
			MDF: StatGrowth{Level: 2.2, PointGrowth: PointGrowth{WIS: 2.2}},
			SPD: StatGrowth{Level: 1.6, PointGrowth: PointGrowth{AGI: 1.8}},
			HP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{CON: 2}}, Base: 180, LevelPower: 0.35, Growth: PointGrowth{CON: 2}, Multiplier: 1},
			SP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{WIS: 2}}, Base: 94, LevelPower: 0.3, Growth: PointGrowth{WIS: 3.2}, Multiplier: 1},
		},
		Water: ElementGrowth{
			ATK: StatGrowth{Level: 1.4, PointGrowth: PointGrowth{STR: 2}},
			DEF: StatGrowth{Level: 2, PointGrowth: PointGrowth{CON: 1.75}},
			MAT: StatGrowth{Level: 1.4, PointGrowth: PointGrowth{INT: 2}},
			MDF: StatGrowth{Level: 2, PointGrowth: PointGrowth{WIS: 2.2}},
			SPD: StatGrowth{Level: 1.6, PointGrowth: PointGrowth{AGI: 1.8}},
			HP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{CON: 2}}, Base: 180, LevelPower: 0.35, Growth: PointGrowth{CON: 2}, Multiplier: 1},
			SP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{WIS: 2}}, Base: 94, LevelPower: 0.3, Growth: PointGrowth{WIS: 3.2}, Multiplier: 1},
		},
		Fire: ElementGrowth{
			ATK: StatGrowth{Level: 2, PointGrowth: PointGrowth{STR: 2}},
			DEF: StatGrowth{Level: 2, PointGrowth: PointGrowth{CON: 1.75}},
			MAT: StatGrowth{Level: 1.6, PointGrowth: PointGrowth{INT: 2}},
			MDF: StatGrowth{Level: 2, PointGrowth: PointGrowth{WIS: 2.2}},
			SPD: StatGrowth{Level: 1.6, PointGrowth: PointGrowth{AGI: 1.8}},
			HP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{CON: 2}}, Base: 180, LevelPower: 0.35, Growth: PointGrowth{CON: 2}, Multiplier: 1},
			SP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{WIS: 2}}, Base: 94, LevelPower: 0.3, Growth: PointGrowth{WIS: 3.2}, Multiplier: 1},
		},
		Wind: ElementGrowth{
			ATK: StatGrowth{Level: 1.4, PointGrowth: PointGrowth{STR: 2}},
			DEF: StatGrowth{Level: 2, PointGrowth: PointGrowth{CON: 1.75}},
			MAT: StatGrowth{Level: 1.4, PointGrowth: PointGrowth{INT: 2}},
			MDF: StatGrowth{Level: 2, PointGrowth: PointGrowth{WIS: 2.2}},
			SPD: StatGrowth{Level: 2.1, PointGrowth: PointGrowth{AGI: 1.8}},
			HP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{CON: 2}}, Base: 180, LevelPower: 0.35, Growth: PointGrowth{CON: 2}, Multiplier: 1},
			SP:  VitalGrowth{StatGrowth: StatGrowth{Level: 1, PointGrowth: PointGrowth{WIS: 2}}, Base: 94, LevelPower: 0.3, Growth: PointGrowth{WIS: 3.2}, Multiplier: 1},
		},
	}
}
