package assetsql

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestXaolanVisibilityRevisionPreservesEdits(t *testing.T) {
	raw, err := os.ReadFile("testdata/xaolan_visibility_v45.json")
	if err != nil {
		t.Fatal(err)
	}
	var historical []assets.StoryRule
	if err := json.Unmarshal(raw, &historical); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"stock", "edited", "removed", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			defs := assets.DefaultStoryDefinitions()
			var rules []assets.StoryRule
			for _, r := range defs.Rules {
				if r.Kind != assets.StoryVisitVisibility {
					rules = append(rules, r)
				}
			}
			defs.Rules = rules
			if scenario != "removed" && scenario != "empty" {
				for _, r := range historical {
					if scenario == "edited" {
						r.Text = "Operator override"
					}
					defs.Rules = append(defs.Rules, r)
				}
			}
			if scenario == "empty" {
				defs.Rules = nil
			}
			db := storyReadFixture(t, defs)
			for range 2 {
				if err := migrateStoryRevision(db, nativeQuestRevisionV46); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readStory(db)
			if err != nil {
				t.Fatal(err)
			}
			want := defs
			if scenario == "stock" {
				want = assets.DefaultStoryDefinitions()
				// v46 appends its visit policy after preexisting rules, preserving
				// their precedence even when newer defaults include later families.
				var existing, additions []assets.StoryRule
				for _, rule := range want.Rules {
					if rule.Kind == assets.StoryVisitVisibility {
						additions = append(additions, rule)
					} else {
						existing = append(existing, rule)
					}
				}
				want.Rules = append(existing, additions...)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("Xaolan revision changed unrelated or edited content")
			}
		})
	}
}
