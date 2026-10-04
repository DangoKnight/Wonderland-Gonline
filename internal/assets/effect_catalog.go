package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const EffectCatalogVersion = 1
const MaxEffectIdentifierBytes = 64
const EffectCatalogFormat = "skill-effect-definitions"
const EffectCatalogAsset = "skill_effects.json"

type EffectDefinition struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Effects []SkillEffect `json:"effects"`
}

type EffectCatalog struct {
	SchemaVersion int                `json:"schema_version"`
	Format        string             `json:"format"`
	Source        string             `json:"source"`
	SourceSHA256  string             `json:"source_sha256"`
	Policy        string             `json:"policy"`
	Records       []EffectDefinition `json:"records"`
}

func ParseEffectCatalog(raw []byte) (map[string]EffectDefinition, error) {
	var doc EffectCatalog
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	if doc.SchemaVersion != EffectCatalogVersion || doc.Format != EffectCatalogFormat {
		return nil, fmt.Errorf("unsupported effect catalog")
	}
	result := map[string]EffectDefinition{}
	for _, d := range doc.Records {
		if d.ID == "" || len(d.ID) > MaxEffectIdentifierBytes || strings.TrimSpace(d.ID) != d.ID {
			return nil, fmt.Errorf("invalid effect identifier %q", d.ID)
		}
		if strings.TrimSpace(d.Name) == "" {
			return nil, fmt.Errorf("effect %s: missing name", d.ID)
		}
		if _, ok := result[d.ID]; ok {
			return nil, fmt.Errorf("duplicate effect %s", d.ID)
		}
		if d.Effects == nil {
			return nil, fmt.Errorf("effect %s: missing effects", d.ID)
		}
		if err := ValidateSkillEffects(d.Effects); err != nil {
			return nil, fmt.Errorf("effect %s: %w", d.ID, err)
		}
		result[d.ID] = d
	}
	return result, nil
}

// Runtime effects come exclusively from explicit SQL inline effects or references.
// An empty reference list deliberately disables effects for that skill.
func ResolveSkillEffects(s *Skill, definitions map[string]EffectDefinition) error {
	if s.EffectRefs != nil {
		if len(s.EffectRefs) > MaxSkillEffects {
			return fmt.Errorf("skill %d: too many effect references", s.ID)
		}
		if s.Effects != nil {
			return fmt.Errorf("skill %d: effects and effect_refs cannot coexist", s.ID)
		}
		s.Effects = []SkillEffect{}
		seen := map[string]bool{}
		for _, id := range s.EffectRefs {
			d, ok := definitions[id]
			if !ok || seen[id] {
				return fmt.Errorf("skill %d: missing or duplicate effect reference %s", s.ID, id)
			}
			seen[id] = true
			// Copy mutable effect fields without a serialization round trip.
			cloned := append([]SkillEffect(nil), d.Effects...)
			for i := range cloned {
				cloned[i].Modifiers = append([]StatModifier(nil), cloned[i].Modifiers...)
				if cloned[i].PeriodicDamage != nil {
					damage := *cloned[i].PeriodicDamage
					cloned[i].PeriodicDamage = &damage
				}
			}
			s.Effects = append(s.Effects, cloned...)
		}
	} else if s.Effects == nil {
		return fmt.Errorf("skill %d: missing effects or effect_refs; regenerate skills and rebuild the assets database", s.ID)
	}
	return ValidateSkillEffects(s.Effects)
}
