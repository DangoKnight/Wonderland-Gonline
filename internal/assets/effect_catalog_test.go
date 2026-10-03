package assets

import (
	"encoding/json"
	"testing"
)

func TestEffectReferencesResolveAndRemainIndependent(t *testing.T) {
	raw := []byte(`{"schema_version":1,"format":"skill-effect-definitions","records":[{"id":"guard","name":"Guard","effects":[{"target":"ally","rounds":4,"modifiers":[{"stat":"def","percent":10},{"stat":"mdef","percent":10}]}]}]}`)
	definitions, err := ParseEffectCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := Skill{ID: 50000, EffectRefs: []string{"guard"}}
	if err := ResolveSkillEffects(&s, definitions); err != nil {
		t.Fatal(err)
	}
	if len(s.Effects) != 1 || s.Effects[0].Modifiers[1].Percent != 10 {
		t.Fatal(s)
	}
	s.Effects[0].Modifiers[0].Percent = 80
	if definitions["guard"].Effects[0].Modifiers[0].Percent != 10 {
		t.Fatal("shared definition was mutated")
	}
	for _, s := range []Skill{{ID: 1, EffectRefs: []string{"missing"}}, {ID: 2, EffectRefs: []string{"guard", "guard"}}, {ID: 3, EffectRefs: []string{"guard"}, Effects: []SkillEffect{}}} {
		if ResolveSkillEffects(&s, definitions) == nil {
			t.Fatal("invalid reference accepted", s)
		}
	}
	disabled := Skill{ID: 4, EffectLayer: 19, NativeEffectCode52: 52, EffectRefs: []string{}}
	if err := ResolveSkillEffects(&disabled, definitions); err != nil || len(disabled.Effects) != 0 {
		t.Fatal("empty refs did not disable native effects", disabled, err)
	}
}

func TestEffectCatalogRejectsInvalidDefinitions(t *testing.T) {
	for _, effects := range []string{`null`, `[{"target":"opponent","rounds":4}]`, `[{"target":"ally","rounds":0,"block_actions":true}]`, `[{"target":"ally","rounds":1,"modifiers":[{"stat":"typo"}]}]`} {
		raw := []byte(`{"schema_version":1,"format":"skill-effect-definitions","records":[{"id":"guard","effects":` + effects + `}]}`)
		if _, err := ParseEffectCatalog(raw); err == nil {
			t.Fatal("invalid effect catalog accepted", effects)
		}
	}
	var doc EffectCatalog
	json.Unmarshal([]byte(`{"schema_version":1,"format":"skill-effect-definitions","records":[{"id":"same","effects":[]},{"id":"same","effects":[]}]}`), &doc)
	raw, _ := json.Marshal(doc)
	if _, err := ParseEffectCatalog(raw); err == nil {
		t.Fatal("duplicate definition accepted")
	}
}
