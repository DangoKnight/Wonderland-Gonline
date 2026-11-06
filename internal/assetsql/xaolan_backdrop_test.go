package assetsql

import (
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestXaolanForestBackdropRevisionPreservesEdits(t *testing.T) {
	const policyID = "north_island_xaolan_battle_background"
	for _, scenario := range []string{"missing", "edited", "removed_family", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			defs := historicalBackdropDefaults()
			var rules []assets.StoryRule
			for _, r := range defs.Rules {
				if r.ID == policyID {
					if scenario != "edited" {
						continue
					}
					r.Value = 91
				}
				if scenario == "removed_family" && r.ID == "robbery_show_actors" {
					continue
				}
				rules = append(rules, r)
			}
			if scenario == "empty" {
				rules = nil
			}
			defs.Rules = rules
			db := storyReadFixture(t, defs)
			for range 2 {
				if err := migrateStoryRevision(db, nativeQuestRevisionV47); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readStory(db)
			if err != nil {
				t.Fatal(err)
			}
			want := defs
			if scenario == "missing" {
				want.Rules = append([]assets.StoryRule(nil), defs.Rules...)
				for _, r := range historicalBackdropDefaults().Rules {
					if r.ID == policyID {
						want.Rules = append(want.Rules, r)
					}
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("forest backdrop migration overwrote edited or removed content")
			}
		})
	}
}
