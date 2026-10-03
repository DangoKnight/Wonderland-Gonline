package assets

import "testing"

func TestNativeConversionPolicyIsDataDriven(t *testing.T) {
	// Previously unknown layer/code: adding a JSON rule needs no Go branch.
	rules, err := ParseNativeEffectRules([]byte(`{"schema_version":1,"policy":"test-policy","rules":[{"name":"Custom","layer":201,"code":202,"effects":[{"target":"self","rounds":2,"modifiers":[{"stat":"atk","percent":37}],"periodic_damage":{"flat":9}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	a := Skill{ID: 50000, EffectLayer: 201, NativeEffectCode52: 202, NativeRounds51: 3}
	if err := rules.Populate(&a); err != nil {
		t.Fatal(err)
	}
	if len(a.Effects) != 1 || a.Effects[0].Rounds != 5 || a.Effects[0].Modifiers[0].Percent != 37 {
		t.Fatal(a)
	}
	a.Effects[0].Modifiers[0].Percent = 100
	a.Effects[0].PeriodicDamage.Flat = 100
	b := Skill{EffectLayer: 201, NativeEffectCode52: 202}
	if err := rules.Populate(&b); err != nil {
		t.Fatal(err)
	}
	if b.Effects[0].Modifiers[0].Percent != 37 || b.Effects[0].PeriodicDamage.Flat != 9 {
		t.Fatal("conversion mutated shared templates", b)
	}
	b.Effects = nil
	b.EffectRefs = []string{}
	if err := rules.Populate(&b); err != nil || b.Effects != nil {
		t.Fatal("references overridden", b, err)
	}
	a.Effects = nil
	a.NativeRounds51 = 255
	if err := rules.Populate(&a); err == nil {
		t.Fatal("invalid converted duration accepted")
	}
}

func TestNativeConversionPolicyRejectsInvalidRules(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":2,"policy":"test","rules":[]}`,
		`{"schema_version":1,"policy":"","rules":[]}`,
		`{"schema_version":1,"policy":"test","rules":[{"name":"A","layer":1,"code":2,"effects":null}]}`,
		`{"schema_version":1,"policy":"test","rules":[{"name":"A","layer":1,"code":2,"effects":[]},{"name":"B","layer":1,"code":2,"effects":[]}]}`,
		`{"schema_version":1,"policy":"test","rules":[{"name":"A","layer":1,"code":2,"effects":[{"target":"enemy","rounds":0,"block_actions":true}]}]}`,
		`{"schema_version":1,"policy":"test","rules":[],"typo":true}`,
		`{"schema_version":1,"policy":"test","rules":[]} {}`,
	} {
		if _, err := ParseNativeEffectRules([]byte(raw)); err == nil {
			t.Fatal("invalid conversion policy accepted", raw)
		}
	}
}

func TestRuntimeResolutionDoesNotUseNativeConversionPolicy(t *testing.T) {
	s := Skill{ID: 50000, EffectLayer: 19, NativeEffectCode52: 60}
	if err := ResolveSkillEffects(&s, nil); err == nil || s.Effects != nil {
		t.Fatal("missing SQL effect metadata gained native effects", s, err)
	}
	s.Effects = []SkillEffect{{Target: EffectSelf, Rounds: 1, Modifiers: []StatModifier{{Stat: EffectSPD, Flat: 7}}}}
	if err := ResolveSkillEffects(&s, nil); err != nil || s.Effects[0].Modifiers[0].Flat != 7 {
		t.Fatal("SQL data was overridden", s, err)
	}
}
