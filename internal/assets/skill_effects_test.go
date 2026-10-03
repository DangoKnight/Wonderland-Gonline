package assets

import (
	"encoding/binary"
	"testing"
)

func TestNativeEffectProjectionDoesNotUseSkillIDOrName(t *testing.T) {
	for _, id := range []uint16{1, 50000} {
		raw := make([]byte, 148)
		for i, c := range []byte("unrelated name") {
			raw[20-i] = c
		}
		binary.LittleEndian.PutUint16(raw[22:], (id+4)^0x6ea0)
		raw[29] = (15 + 4) ^ 0xfd
		raw[51] = (3 + 4) ^ 0xfd
		raw[52] = (172 + 4) ^ 0xfd
		skills, err := ParseSkills(raw)
		if err != nil {
			t.Fatal(err)
		}
		s := skills[id]
		if len(s.Effects) != 1 || s.Effects[0].Target != EffectEnemy || s.Effects[0].Rounds != 4 || len(s.Effects[0].Modifiers) != 4 {
			t.Fatal("native effect class not projected", s)
		}
		if s.Effects[0].Modifiers[0].Stat != EffectATK || s.Effects[0].Modifiers[0].Percent != -50 {
			t.Fatal("wrong reduction")
		}
	}
}
func TestAuthoredEffectsOverrideCompatibilityProjection(t *testing.T) {
	s := Skill{EffectLayer: 19, NativeEffectCode52: 60, Effects: []SkillEffect{{Target: EffectSelf, Rounds: 7, Modifiers: []StatModifier{{Stat: EffectSPD, Flat: 12}}}}}
	PopulateSkillEffects(&s)
	if len(s.Effects) != 1 || s.Effects[0].Rounds != 7 || s.Effects[0].Modifiers[0].Flat != 12 {
		t.Fatal("authored effects replaced")
	}
	s.Effects = []SkillEffect{}
	PopulateSkillEffects(&s)
	if len(s.Effects) != 0 {
		t.Fatal("explicit empty override ignored")
	}
}
func TestEffectDefinitionValidation(t *testing.T) {
	for _, e := range []SkillEffect{
		{Target: "unknown", Rounds: 1, Modifiers: []StatModifier{{Stat: EffectSPD}}},
		{Target: EffectAlly, Rounds: 0, Modifiers: []StatModifier{{Stat: EffectSPD}}},
		{Target: EffectAlly, Rounds: 256, Modifiers: []StatModifier{{Stat: EffectSPD}}},
		{Target: EffectAlly, Rounds: 1},
		{Target: EffectAlly, Rounds: 1, Modifiers: []StatModifier{{Stat: "unknown"}}},
	} {
		if ValidateSkillEffects([]SkillEffect{e}) == nil {
			t.Fatal("invalid definition accepted", e)
		}
	}
}

func TestNativeControlEffectsDoNotUseNamesOrIDs(t *testing.T) {
	for _, tc := range []struct {
		layer, code                   byte
		block, wake, periodic, visual bool
		chance                        int
	}{
		{3, 1, true, false, false, false, 0}, {3, 3, true, false, false, false, 0}, {3, 4, true, false, false, false, 0},
		{3, 6, true, true, false, false, 0}, {3, 7, true, false, false, false, 0}, {3, 8, true, false, false, false, 0},
		{15, 177, true, false, false, false, 0}, {15, 178, true, false, false, false, 0},
		{15, 171, false, false, true, false, 0}, {15, 173, false, false, false, false, 50}, {19, 53, false, false, false, true, 0},
	} {
		s := Skill{ID: 50000, Name: "unrelated name", EffectLayer: tc.layer, NativeEffectCode52: tc.code, NativeRounds51: 1}
		PopulateSkillEffects(&s)
		if len(s.Effects) != 1 {
			t.Fatal("native control class not projected", tc)
		}
		e := s.Effects[0]
		if e.BlockActions != tc.block || e.BreakOnDamage != tc.wake || (e.PeriodicDamage != nil) != tc.periodic || (e.Presentation != "") != tc.visual || e.AllyAttackChancePercent != tc.chance {
			t.Fatal("wrong behavioral properties", tc, e)
		}
		if err := ValidateSkillEffects(s.Effects); err != nil {
			t.Fatal(err)
		}
	}
	// Cure layer shares a code with the debuff, but must not inflict it.
	cure := Skill{EffectLayer: 5, NativeEffectCode52: 173}
	PopulateSkillEffects(&cure)
	if len(cure.Effects) != 0 {
		t.Fatal("cure classified as a debuff")
	}
}

func TestNativeVanishProjectsProtectionProperties(t *testing.T) {
	s := Skill{ID: 50000, Name: "unrelated", EffectLayer: 19, NativeEffectCode52: 53, NativeRounds51: 2}
	PopulateSkillEffects(&s)
	e := s.Effects[0]
	if !e.MissPhysicalAttacks || !e.MissAreaAttacks || e.Target != EffectAlly || e.Rounds != 3 {
		t.Fatal("Vanish protection not projected", e)
	}
	for _, e := range []SkillEffect{
		{Target: EffectAlly, Rounds: 1, MissPhysicalAttacks: true},
		{Target: EffectAlly, Rounds: 1, MissAreaAttacks: true},
	} {
		if err := ValidateSkillEffects([]SkillEffect{e}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeAreaPatternsAndOverride(t *testing.T) {
	raw := make([]byte, 148)
	raw[20] = 'x'
	binary.LittleEndian.PutUint16(raw[22:], (50000+4)^0x6ea0)
	for i, p := range []byte{1, 1, 2, 3} {
		raw[109+i] = (p + 4) ^ 0xfd
	}
	skills, err := ParseSkills(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := skills[50000]
	for grade := 1; grade <= 10; grade++ {
		if s.IsAreaAttack(grade) != (grade >= 7) {
			t.Fatal("pattern grade band", grade, s)
		}
	}
	single := false
	s.AreaAttack = &single
	if s.IsAreaAttack(10) {
		t.Fatal("single override ignored")
	}
	area := true
	s.AreaAttack = &area
	if !s.IsAreaAttack(1) {
		t.Fatal("area override ignored")
	}
}

func TestNativeStoneWallProjectsPhysicalProtection(t *testing.T) {
	// Use an unrelated ID/name: behavior comes from the native layer/code pair.
	s := Skill{ID: 50000, Name: "unrelated", EffectLayer: 4, NativeEffectCode52: 101, NativeRounds51: 3}
	PopulateSkillEffects(&s)
	if len(s.Effects) != 1 {
		t.Fatal("Stone Wall effect missing", s)
	}
	e := s.Effects[0]
	if e.Target != EffectAlly || e.Rounds != 4 || !e.MissPhysicalAttacks || e.MissAreaAttacks || e.BlockActions || len(e.Modifiers) != 0 {
		t.Fatal("wrong Stone Wall protection", e)
	}
	if err := ValidateSkillEffects(s.Effects); err != nil {
		t.Fatal(err)
	}
	// A shared effect code in an unrelated layer must not gain immunity.
	unrelated := Skill{EffectLayer: 2, NativeEffectCode52: 101}
	PopulateSkillEffects(&unrelated)
	if len(unrelated.Effects) != 0 {
		t.Fatal("unrelated layer gained protection")
	}
}

func TestNativeWaterShieldProjectsOnlyMagicalDamageReduction(t *testing.T) {
	s := Skill{ID: 50000, Name: "unrelated", EffectLayer: 4, NativeEffectCode52: 105, NativeRounds51: 4}
	PopulateSkillEffects(&s)
	if len(s.Effects) != 1 {
		t.Fatal("Water Shield effect missing")
	}
	e := s.Effects[0]
	if e.Target != EffectAlly || e.Rounds != 5 || len(e.Modifiers) != 1 || e.Modifiers[0].Stat != EffectMagicalDamageTaken || e.Modifiers[0].Percent != -50 {
		t.Fatal("wrong Water Shield modifier", e)
	}
	if err := ValidateSkillEffects(s.Effects); err != nil {
		t.Fatal(err)
	}
	generic := Skill{EffectLayer: 19, NativeEffectCode52: 52}
	PopulateSkillEffects(&generic)
	if generic.Effects[0].Modifiers[0].Stat != EffectDEF {
		t.Fatal("defense buff scope incorrect")
	}
}

func TestNativeShieldDefenseProjectsDefenseStats(t *testing.T) {
	s := Skill{ID: 50000, Name: "arbitrary", SP: 15, EffectLayer: 19, NativeEffectCode52: 52, NativeRounds51: 3}
	PopulateSkillEffects(&s)
	if len(s.Effects) != 1 {
		t.Fatal("missing Shield Defense effect")
	}
	e := s.Effects[0]
	if e.Target != EffectAlly || e.Rounds != 4 || e.StackGroup != "protection_buff" || len(e.Modifiers) != 2 || e.Modifiers[0].Stat != EffectDEF || e.Modifiers[1].Stat != EffectMDEF || e.Modifiers[0].Percent != 10 || e.Modifiers[1].Percent != 10 {
		t.Fatal("incorrect Shield Defense projection", e)
	}
	if err := ValidateSkillEffects(s.Effects); err != nil {
		t.Fatal(err)
	}
}
