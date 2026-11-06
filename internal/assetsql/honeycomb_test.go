package assetsql

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestHoneycombMigrationPreservesEdits(t *testing.T) {
	for _, version := range []int{40, 41, 42} {
		for _, scenario := range []string{"stock", "edited", "empty", "removed"} {
			t.Run(fmt.Sprintf("%d/%s", version, scenario), func(t *testing.T) {
				defs := assets.DefaultStoryDefinitions()
				// These historical schemas predate backdrop/visit policies. Exercise
				// their honey revision independently of later content batches.
				var baseline []assets.StoryRule
				for _, r := range defs.Rules {
					if r.Kind != assets.StoryBattleBackground && r.Kind != assets.StoryVisitVisibility {
						baseline = append(baseline, r)
					}
				}
				defs.Rules = baseline
				expected := defs
				if version == 40 {
					// The missing rule is appended after preexisting policies. Preserve
					// their order even when this fixture includes newer policy families.
					var ordered []assets.StoryRule
					var addition assets.StoryRule
					for _, rule := range expected.Rules {
						if rule.ID == "honeycomb_start_bee_battle" {
							addition = rule
						} else {
							ordered = append(ordered, rule)
						}
					}
					expected.Rules = append(ordered, addition)
				}
				var old []assets.StoryRule
				raw, err := os.ReadFile("testdata/honey_story_v42.json")
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(raw, &old); err != nil {
					t.Fatal(err)
				}
				var rules []assets.StoryRule
				for _, r := range defs.Rules {
					if version == 40 && r.ID == "honeycomb_start_bee_battle" {
						continue
					}
					if version == 40 && r.ID == "always_visible_kelan" {
						r.Scope.Actors = []uint16{17, 10, 31, 29}
					}
					if version == 40 && r.ID == "kelan_scenery" {
						r.Scope.Actors = []uint16{17, 10, 31}
					}
					rules = append(rules, r)
				}
				var edited []assets.StoryRule
				for _, r := range old {
					if version < 42 && r.ID == "honey_patient_pose_until_hand_in" {
						continue
					}
					if version == 40 {
						if r.ID == "honeycomb_battle_completion" {
							continue
						}
						r.Scope.Actors = []uint16{18, 19}
						if r.ID == "wine_actors_visible" {
							r.Gates = []assets.StoryGate{{Source: "mark_state", ID: 13023, Op: "ne", Value: 2}}
						}
					}
					if scenario == "edited" {
						r.Text = "Operator override"
						edited = append(edited, r)
					}
					if scenario != "removed" {
						rules = append(rules, r)
					}
				}
				if scenario == "empty" {
					rules = nil
				}
				defs.Rules = rules
				db := storyReadFixture(t, defs)
				for range 2 {
					if err := migrateStoryRevision(db, nativeQuestRevisionV43); err != nil {
						t.Fatal(err)
					}
				}
				got, err := readStory(db)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "stock" {
					if !reflect.DeepEqual(got, expected) {
						t.Fatal("stock policy did not return to native progression")
					}
				} else if scenario == "edited" {
					for _, want := range edited {
						found := false
						for _, r := range got.Rules {
							found = found || sameStoryRule(r, want)
						}
						if !found {
							t.Fatal("operator override lost", want.ID)
						}
					}
				} else if scenario == "removed" && version == 40 {
					for _, r := range got.Rules {
						if r.ID == "honeycomb_start_bee_battle" {
							t.Fatal("removed honey family recreated")
						}
					}
				} else if !reflect.DeepEqual(got, defs) {
					t.Fatal("empty or removed policies recreated")
				}
			})
		}
	}
}
