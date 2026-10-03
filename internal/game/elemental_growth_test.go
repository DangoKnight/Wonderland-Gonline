package game

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"
)

func TestElementalGrowthIndependentCoefficients(t *testing.T) {
	for _, element := range []byte{Earth, Water, Fire, Wind} {
		g := DefaultElementalGrowth()
		custom := ElementGrowth{
			ATK: StatGrowth{Level: 3, PointGrowth: PointGrowth{STR: 4, CON: 1}},
			DEF: StatGrowth{Level: 4, PointGrowth: PointGrowth{CON: 5, INT: 1}},
			MAT: StatGrowth{Level: 5, PointGrowth: PointGrowth{INT: 6, WIS: 1}},
			MDF: StatGrowth{Level: 6, PointGrowth: PointGrowth{WIS: 7, AGI: 1}},
			SPD: StatGrowth{Level: 7, PointGrowth: PointGrowth{AGI: 8, STR: 1}},
			HP:  VitalGrowth{StatGrowth: StatGrowth{Level: 2, PointGrowth: PointGrowth{CON: 3}}, Base: 100, LevelPower: 1, Growth: PointGrowth{CON: 4}, Multiplier: 1.5},
			SP:  VitalGrowth{StatGrowth: StatGrowth{Level: 3, PointGrowth: PointGrowth{WIS: 4}}, Base: 50, LevelPower: 0, Growth: PointGrowth{WIS: 5}, Multiplier: 2},
		}
		switch element {
		case Earth:
			g.Earth = custom
		case Water:
			g.Water = custom
		case Fire:
			g.Fire = custom
		case Wind:
			g.Wind = custom
		}
		c := Character{Level: 2, Element: element, Base: Attributes{Strength: 3, Constitution: 4, Intelligence: 5, Wisdom: 6, Agility: 7}}
		items := map[uint16]ItemDefinition{1: {ID: 1, Status: [2]uint16{207, 208}, Values: [2]int32{110, 120}}}
		c.Equipment[0] = Item{ID: 1, Count: 1}
		want := Combat{MaxHP: 182, MaxSP: 190, ATK: 22, DEF: 33, MAT: 46, MDF: 61, SPD: 73}
		if got := c.Combat(items, g); got != want {
			t.Fatalf("element %d: got %+v, want %+v", element, got, want)
		}
		c.Refill(items, g)
		if c.HP != 182 || c.SP != 190 || c.MaxHP != 182 || c.MaxSP != 190 {
			t.Fatal("refill ignored growth", c)
		}
	}
}

func TestElementalGrowthVitalDefaults(t *testing.T) {
	for _, element := range []byte{Earth, Water, Fire, Wind} {
		for _, tc := range []struct {
			level  byte
			attr   uint16
			hp, sp int32
		}{
			{1, 0, 181, 95}, {1, 5, 201, 121}, {37, 22, 417, 383}, {40, 20, 405, 368},
		} {
			c := Character{Level: tc.level, Element: element, Base: Attributes{Constitution: tc.attr, Wisdom: tc.attr}}
			if got := c.Combat(nil); got.MaxHP != tc.hp || got.MaxSP != tc.sp {
				t.Fatalf("element %d level %d attr %d: %+v", element, tc.level, tc.attr, got)
			}
		}
	}
}

func TestElementalGrowthCreationAndRecalculation(t *testing.T) {
	g := DefaultElementalGrowth()
	g.Fire.HP.Base = 300
	g.Fire.SP.Base = 150
	c, err := NewCharacter(10001, 1, "Player", Appearance{Body: 4, Element: Fire, Base: Attributes{5, 5, 5, 5, 5}}, nil, starterTestItems(), time.Unix(0, 0), g)
	if err != nil {
		t.Fatal(err)
	}
	if c.HP != 329 || c.SP != 177 {
		t.Fatal("creation ignored growth", c.HP, c.SP)
	}
	c.HP, c.SP = 100, 50
	g.Fire.HP.Base = 50
	g.Fire.SP.Base = 0
	if !c.RecalculateVitals(nil, g) || c.HP != 79 || c.SP != 27 || c.MaxHP != 79 || c.MaxSP != 27 {
		t.Fatal("growth change did not clamp", c)
	}
	if c.RecalculateVitals(nil, g) {
		t.Fatal("unchanged vitals reported changed")
	}
	g.Fire.HP.Base = 300
	g.Fire.SP.Base = 150
	c.RecalculateVitals(nil, g)
	if c.HP != 79 || c.SP != 27 {
		t.Fatal("recalculation healed character")
	}
}

func TestElementalGrowthNativeContributions(t *testing.T) {
	c := Character{Level: 10, Element: Fire, Base: Attributes{Strength: 5, Constitution: 5, Wisdom: 5, Agility: 5, Intelligence: 5}}
	g := DefaultElementalGrowth()
	g.Fire.ATK.Level = 3
	g.Fire.ATK.STR = 3
	g.Fire.HP.Multiplier = 2
	g.Fire.SP.Base = 0 // Exercise signed negative adjustment in the uint32 wire field.
	packets := c.StatPackets(nil, g)
	got := map[byte]int32{}
	for _, p := range packets {
		got[p[2]] = int32(binary.LittleEndian.Uint32(p[4:8]))
	}
	if got[StatAttack] != 25 || got[StatHPBonus] != 43 || got[StatSPBonus] != -94 {
		t.Fatal(got)
	}
	c.Element = Water
	got = map[byte]int32{}
	for _, p := range c.StatPackets(nil) {
		got[p[2]] = int32(binary.LittleEndian.Uint32(p[4:8]))
	}
	if got[StatMagicDefense] != 11 || got[StatSPBonus] != 0 {
		t.Fatal("native elemental corrections", got)
	}
}

func TestElementalGrowthValidation(t *testing.T) {
	if err := DefaultElementalGrowth().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ElementalGrowth){
		func(g *ElementalGrowth) { g.Fire.ATK.Level = -1 },
		func(g *ElementalGrowth) { g.Earth.HP.Multiplier = 0 },
		func(g *ElementalGrowth) { g.Water.SP.LevelPower = 3 },
		func(g *ElementalGrowth) { g.Wind.SPD.AGI = math.NaN() },
		func(g *ElementalGrowth) { g.Fire.MAT.INT = math.Inf(1) },
		func(g *ElementalGrowth) { g.Water.SP.Growth.WIS = 1000; g.Water.SP.LevelPower = 2 },
	} {
		g := DefaultElementalGrowth()
		change(&g)
		if g.Validate() == nil {
			t.Fatal("accepted invalid growth", g)
		}
	}
}

func TestElementalGrowthMatchesFormulaExport(t *testing.T) {
	raw, err := os.ReadFile("../../data/formula_data.json")
	if err != nil {
		t.Fatal(err)
	}
	var formula struct {
		Coefficients []float64 `json:"coefficients"`
		Bases        []uint16  `json:"level_gates"`
	}
	if err = json.Unmarshal(raw, &formula); err != nil {
		t.Fatal(err)
	}
	if len(formula.Coefficients) != 45 || len(formula.Bases) != 21 {
		t.Fatal("unexpected formula export layout")
	}
	g := DefaultElementalGrowth()
	for _, element := range []byte{Earth, Water, Fire, Wind} {
		e := g.ForElement(element)
		for _, tc := range []struct {
			name        string
			v           VitalGrowth
			index, base int
		}{
			{"HP", e.HP, 31, 0}, {"SP", e.SP, 35, 1},
		} {
			attr, growth := tc.v.CON, tc.v.Growth.CON
			if tc.name == "SP" {
				attr, growth = tc.v.WIS, tc.v.Growth.WIS
			}
			want := formula.Coefficients[tc.index : tc.index+4]
			if tc.v.Level != want[0] || growth != want[1] || tc.v.LevelPower != want[2] || attr != want[3] || tc.v.Base != float64(formula.Bases[tc.base]) || tc.v.Multiplier != 1 {
				t.Fatalf("element %d %s differs from Formula.Dat: %+v", element, tc.name, tc.v)
			}
		}
	}
}

func TestElementalGrowthStandardReferenceCoefficients(t *testing.T) {
	// Independent expectations transcribed from the saved WLRI Japanese wiki's
	// elemental parameter table, including Earth MDF and the fractional points.
	for _, tc := range []struct {
		element                 byte
		atk, def, mat, mdf, spd float64
	}{
		{Earth, 1.4, 2.6, 1.4, 2.2, 1.6},
		{Fire, 2, 2, 1.6, 2, 1.6},
		{Water, 1.4, 2, 1.4, 2, 1.6},
		{Wind, 1.4, 2, 1.4, 2, 2.1},
	} {
		e := DefaultElementalGrowth().ForElement(tc.element)
		if e.ATK != (StatGrowth{Level: tc.atk, PointGrowth: PointGrowth{STR: 2}}) ||
			e.DEF != (StatGrowth{Level: tc.def, PointGrowth: PointGrowth{CON: 1.75}}) ||
			e.MAT != (StatGrowth{Level: tc.mat, PointGrowth: PointGrowth{INT: 2}}) ||
			e.MDF != (StatGrowth{Level: tc.mdf, PointGrowth: PointGrowth{WIS: 2.2}}) ||
			e.SPD != (StatGrowth{Level: tc.spd, PointGrowth: PointGrowth{AGI: 1.8}}) {
			t.Fatalf("element %d differs from reference: %+v", tc.element, e)
		}
	}
}
