package assetsql

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestNativeQuestBackdropRevisionPreservesEdits(t *testing.T) {
	for _, scenario := range []string{"missing", "edited", "removed_family", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			defs := historicalBackdropDefaults()
			raw, err := os.ReadFile("testdata/xaolan_visibility_v45.json")
			if err != nil {
				t.Fatal(err)
			}
			var historical []assets.StoryRule
			if err := json.Unmarshal(raw, &historical); err != nil {
				t.Fatal(err)
			}
			defs.Rules = append(defs.Rules, historical...)
			var rules []assets.StoryRule
			for _, r := range defs.Rules {
				if r.Kind == assets.StoryVisitVisibility || r.ID == "north_island_xaolan_battle_background" {
					continue // This revision predates visit-scoped visibility.
				}
				if r.Kind == assets.StoryBattleBackground {
					if scenario != "edited" {
						continue
					}
					r.Value = 139
				}
				if scenario == "removed_family" && (r.ID == "robbery_hide_companion" || r.ID == "robbery_show_actors") {
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
				if err := migrateStoryRevision(db, nativeQuestRevisionV45); err != nil {
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
					if r.ID == "kelan_xaolan_house_battle_background" {
						want.Rules = append(want.Rules, r)
					}
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("backdrop revision changed edited/removed quest content")
			}
		})
	}
}
