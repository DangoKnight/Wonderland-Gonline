package assets

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

const NativeEffectRulesVersion = 1

// Conversion policy is authored separately from derived data/ outputs. Runtime
// SQL loading never consults it; only native decoders and offline exporters do.
//
//go:embed native_effect_rules.json
var nativeEffectRulesJSON []byte

type NativeEffectRule struct {
	Name  string `json:"name"`
	Layer byte   `json:"layer"`
	Code  byte   `json:"code"`
	// Template rounds are added to native field 51, including the casting round.
	Effects []SkillEffect `json:"effects"`
}

type NativeEffectRules struct {
	SchemaVersion int                `json:"schema_version"`
	Policy        string             `json:"policy"`
	Rules         []NativeEffectRule `json:"rules"`
}

func ParseNativeEffectRules(raw []byte) (NativeEffectRules, error) {
	var rules NativeEffectRules
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rules); err != nil {
		return rules, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return rules, fmt.Errorf("unexpected trailing conversion policy data")
	}
	if rules.SchemaVersion != NativeEffectRulesVersion || strings.TrimSpace(rules.Policy) == "" {
		return rules, fmt.Errorf("unsupported native effect conversion policy")
	}
	seen := map[[2]byte]bool{}
	for _, rule := range rules.Rules {
		key := [2]byte{rule.Layer, rule.Code}
		if strings.TrimSpace(rule.Name) == "" || rule.Effects == nil || seen[key] {
			return rules, fmt.Errorf("invalid or duplicate native effect rule %d/%d", rule.Layer, rule.Code)
		}
		seen[key] = true
		if err := ValidateSkillEffects(rule.Effects); err != nil {
			return rules, fmt.Errorf("rule %s: %w", rule.Name, err)
		}
	}
	return rules, nil
}

func DefaultNativeEffectRules() NativeEffectRules {
	rules, err := ParseNativeEffectRules(nativeEffectRulesJSON)
	if err != nil {
		panic(fmt.Sprintf("invalid embedded native conversion policy: %v", err))
	}
	return rules
}

// Populate resolves layer/code mappings without ability-specific Go branches.
// Explicit references and inline lists, including empty ones, take precedence.
func (rules NativeEffectRules) Populate(s *Skill) error {
	if s.Effects != nil || s.EffectRefs != nil {
		return nil
	}
	for _, rule := range rules.Rules {
		if rule.Layer != s.EffectLayer || rule.Code != s.NativeEffectCode52 {
			continue
		}
		raw, err := json.Marshal(rule.Effects)
		if err != nil {
			return err
		}
		var effects []SkillEffect
		if err := json.Unmarshal(raw, &effects); err != nil {
			return err
		}
		for i := range effects {
			effects[i].Rounds += int(s.NativeRounds51)
		}
		if err := ValidateSkillEffects(effects); err != nil {
			return fmt.Errorf("skill %d: %w", s.ID, err)
		}
		s.Effects = effects
		return nil
	}
	return nil
}

// Parse the embedded policy only when an offline decoder requests it.
var defaultNativeEffectRules = sync.OnceValue(DefaultNativeEffectRules)

// PopulateSkillEffects is retained for offline native decoding and compatibility
// tests. Gameplay resolves only SQL effect references or inline definitions.
func PopulateSkillEffects(s *Skill) error {
	return defaultNativeEffectRules().Populate(s)
}
