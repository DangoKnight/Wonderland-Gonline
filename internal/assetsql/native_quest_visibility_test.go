package assetsql

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"wonderland-gonline/internal/assets"
)

func TestNativeQuestVisibilityRevisionPreservesEdits(t *testing.T) {
	raw, err := os.ReadFile("testdata/lost_dog_story_v43.json")
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
			if scenario != "removed" {
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
				if err := migrateStoryRevision(db, nativeQuestRevisionV44); err != nil {
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
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("visibility migration changed unrelated or edited content")
			}
		})
	}
}
