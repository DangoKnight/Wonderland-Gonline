package assetsql

import (
	"encoding/json"
	"reflect"
	"testing"

	"wonderland-gonline/internal/assets"
)

func TestStoryRevisionPreservesAuthoredPolicies(t *testing.T) {
	parent := assets.StoryRule{ID: "old_policy", Kind: assets.StoryVisibility, Value: 1}
	replace := assets.StoryRule{ID: "actor_policy", Kind: assets.StoryVisibility, Value: 1}
	updated := replace
	updated.Value = 0
	added := assets.StoryRule{ID: "new_link", Kind: assets.StoryContinue}
	revision := storyRevision{
		ID:      "test_revision",
		Remove:  []assets.StoryRule{parent},
		Replace: []storyRuleReplacement{{Before: replace, After: updated}},
		Append:  []storyRuleAddition{{WhenAnyPresent: []string{parent.ID}, Rule: added}},
	}
	raw, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	edited := parent
	edited.Value = 0
	editedLink := added
	editedLink.Value = 1
	for _, tc := range []struct {
		name          string
		before, after []assets.StoryRule
	}{
		{"stock", []assets.StoryRule{parent, replace}, []assets.StoryRule{updated, added}},
		{"edited parent", []assets.StoryRule{edited, replace}, []assets.StoryRule{edited, updated, added}},
		{"removed parent", []assets.StoryRule{replace}, []assets.StoryRule{updated}},
		{"existing link", []assets.StoryRule{parent, added}, []assets.StoryRule{added}},
		{"edited link", []assets.StoryRule{parent, editedLink}, []assets.StoryRule{editedLink}},
		{"empty", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]assets.StoryRule(nil), tc.before...)
			got, _, err := applyStoryRevision(tc.before, raw)
			if err != nil || !reflect.DeepEqual(got, tc.after) {
				t.Fatal("revision result", got, err)
			}
			if !reflect.DeepEqual(original, tc.before) {
				t.Fatal("input catalog mutated")
			}
			again, changed, err := applyStoryRevision(got, raw)
			if err != nil || changed || !reflect.DeepEqual(again, got) {
				t.Fatal("revision not idempotent", err)
			}
		})
	}
}

func TestStoryRevisionRejectsInvalidDocuments(t *testing.T) {
	for _, raw := range []string{
		`{`, `{}`, `{"id":"x","unknown":true}`,
		`{"id":"x"} {"id":"y"}`,
		`{"id":"x","remove":[{}]}`,
		`{"id":"x","replace":[{"before":{"id":"a"},"after":{"id":"b"}}]}`,
		`{"id":"x","append":[{"rule":{"id":"a"}},{"rule":{"id":"a"}}]}`,
		`{"id":"x","append":[{"when_any_present":[""],"rule":{"id":"a"}}]}`,
	} {
		if _, _, err := applyStoryRevision(nil, []byte(raw)); err == nil {
			t.Fatal("accepted invalid revision", raw)
		}
	}
}

func TestXaolanRecruitmentContinuationRevision(t *testing.T) {
	defs := assets.DefaultStoryDefinitions()
	var expected assets.StoryRule
	var old []assets.StoryRule
	for _, r := range defs.Rules {
		if r.ID == "xaolan_recruitment_question_continuation" {
			expected = r
		} else {
			old = append(old, r)
		}
	}
	if expected.ID == "" {
		t.Fatal("default continuation missing")
	}
	migrated, changed, err := applyStoryRevision(old, nativeQuestRevisionV51)
	if err != nil || !changed {
		t.Fatal("migration did not append continuation", err)
	}
	if !sameStoryRule(migrated[len(migrated)-1], expected) {
		t.Fatal("migration differs from fresh defaults")
	}
	edited := expected
	edited.TargetBranch = 5
	old = append(old, edited)
	migrated, changed, err = applyStoryRevision(old, nativeQuestRevisionV51)
	if err != nil || changed || !reflect.DeepEqual(old, migrated) {
		t.Fatal("administrator edit overwritten", err)
	}
	if _, changed, err = applyStoryRevision(nil, nativeQuestRevisionV51); err != nil || changed {
		t.Fatal("empty collection reseeded", err)
	}
}

func TestNoaConversationContinuationRevision(t *testing.T) {
	defs := assets.DefaultStoryDefinitions()
	var before, expected []assets.StoryRule
	for _, r := range defs.Rules {
		if r.ID == "noa_quiz_result_continuation" || r.ID == "noa_quiz_retry_continuation" || r.ID == "noa_recruitment_continuation" {
			expected = append(expected, r)
		} else {
			before = append(before, r)
		}
	}
	got, changed, err := applyStoryRevision(before, nativeQuestRevisionV52)
	if err != nil || !changed || len(expected) != 3 || !reflect.DeepEqual(got[len(before):], expected) {
		t.Fatal("Noa migration differs from new defaults", err)
	}
	got[len(before)].TargetBranch = 3 // Keep administrator customization.
	for _, rules := range [][]assets.StoryRule{got, nil} {
		again, changed, err := applyStoryRevision(rules, nativeQuestRevisionV52)
		if err != nil || changed || !reflect.DeepEqual(again, rules) {
			t.Fatal("revision overwrote existing policies", err)
		}
	}
}
