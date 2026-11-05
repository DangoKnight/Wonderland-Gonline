package assetsql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// storyRevision describes an offline change to SQL-owned modular quest policies.
// Exact before-images preserve administrator edits; append prerequisites are
// evaluated against the original collection, before any policies are removed.
type storyRevision struct {
	ID      string                 `json:"id"`
	Remove  []assets.StoryRule     `json:"remove"`
	Replace []storyRuleReplacement `json:"replace"`
	Append  []storyRuleAddition    `json:"append"`
}

type storyRuleReplacement struct {
	Before assets.StoryRule `json:"before"`
	After  assets.StoryRule `json:"after"`
}

type storyRuleAddition struct {
	WhenAnyPresent []string         `json:"when_any_present"`
	Rule           assets.StoryRule `json:"rule"`
}

func sameStoryRule(a, b assets.StoryRule) bool {
	// Canonical struct encoding treats omitted and empty optional slices alike.
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}

func applyStoryRevision(existing []assets.StoryRule, raw []byte) ([]assets.StoryRule, bool, error) {
	var revision storyRevision
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&revision); err != nil {
		return nil, false, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, false, fmt.Errorf("story revision contains trailing data")
	}
	if revision.ID == "" {
		return nil, false, fmt.Errorf("story revision needs an identity")
	}
	for _, r := range revision.Remove {
		if r.ID == "" {
			return nil, false, fmt.Errorf("story revision %s: removal needs an identity", revision.ID)
		}
	}
	for _, r := range revision.Replace {
		if r.Before.ID == "" || r.Before.ID != r.After.ID {
			return nil, false, fmt.Errorf("story revision %s: replacement must retain its identity", revision.ID)
		}
	}
	additions := map[string]bool{}
	for _, r := range revision.Append {
		if r.Rule.ID == "" || additions[r.Rule.ID] {
			return nil, false, fmt.Errorf("story revision %s: invalid or duplicate addition", revision.ID)
		}
		additions[r.Rule.ID] = true
		for _, id := range r.WhenAnyPresent {
			if id == "" {
				return nil, false, fmt.Errorf("story revision %s: empty prerequisite identity", revision.ID)
			}
		}
	}
	if len(existing) == 0 {
		return existing, false, nil // An intentionally empty collection stays empty.
	}
	original := map[string]bool{}
	for _, r := range existing {
		original[r.ID] = true
	}
	changed := false
	result := make([]assets.StoryRule, 0, len(existing))
	for _, r := range existing {
		remove := false
		for _, before := range revision.Remove {
			if sameStoryRule(r, before) {
				remove = true
				break
			}
		}
		if remove {
			changed = true
			continue
		}
		for _, replacement := range revision.Replace {
			if sameStoryRule(r, replacement.Before) {
				r = replacement.After
				changed = true
				break
			}
		}
		result = append(result, r)
	}
	for _, addition := range revision.Append {
		if original[addition.Rule.ID] {
			continue // Never replace an operator's same-ID policy.
		}
		eligible := len(addition.WhenAnyPresent) == 0
		for _, id := range addition.WhenAnyPresent {
			eligible = eligible || original[id]
		}
		if eligible {
			result = append(result, addition.Rule)
			changed = true
		}
	}
	return result, changed, nil
}

func migrateStoryRevision(tx *gorm.DB, raw []byte) error {
	existing, err := readStoryRules(tx, catalogMetadataID)
	if err != nil {
		return err
	}
	rules, changed, err := applyStoryRevision(existing, raw)
	if err != nil || !changed {
		return err
	}
	if err := tx.Where("story_key = ?", catalogMetadataID).Delete(&StoryRulesRow{}).Error; err != nil {
		return err
	}
	return writeStoryRules(tx, rules, catalogMetadataID)
}
