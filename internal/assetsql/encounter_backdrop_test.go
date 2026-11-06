package assetsql

import (
	"encoding/json"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestEncounterBackdropRevisionPreservesEdits(t *testing.T) {
	ids := map[string]bool{"kelan_village_battle_background": true, "north_island_lakeside_mushroom_battle_background": true}
	testBackdropRevisionPreservesEdits(t, nativeQuestRevisionV48, ids)
}

func TestSeaflowerBackdropRevisionPreservesEdits(t *testing.T) {
	testBackdropRevisionPreservesEdits(t, nativeQuestRevisionV49, map[string]bool{"north_island_seaflower_beach_battle_background": true})
}

func testBackdropRevisionPreservesEdits(t *testing.T, revision []byte, ids map[string]bool) {
	t.Helper()
	for _, scenario := range []string{"missing", "edited", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			defs := historicalBackdropDefaults()
			var rules, additions []assets.StoryRule
			for _, r := range defs.Rules {
				if ids[r.ID] {
					additions = append(additions, r)
					if scenario == "missing" {
						continue
					}
					r.Value = 82
				}
				rules = append(rules, r)
			}
			if scenario == "empty" {
				rules = nil
			}
			defs.Rules = rules
			db := storyReadFixture(t, defs)
			for range 2 {
				if err := migrateStoryRevision(db, revision); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readStory(db)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "missing" {
				defs.Rules = append(defs.Rules, additions...)
			}
			if !reflect.DeepEqual(got, defs) {
				t.Fatal("backdrop revision lost operator edits or was not idempotent")
			}
		})
	}
}

// Historical migration tests must retain the pre-v50 before-images even after
// current defaults no longer contain encounter-specific backdrop policies.
func historicalBackdropDefaults() assets.StoryDefinitions {
	defs := assets.DefaultStoryDefinitions()
	var revision storyRevision
	if err := json.Unmarshal(nativeQuestRevisionV50, &revision); err != nil {
		panic(err)
	}
	defs.Rules = append(defs.Rules, revision.Remove...)
	return defs
}
