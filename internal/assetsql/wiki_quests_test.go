package assetsql

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"reflect"
	"slices"
	"testing"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestQuestBranchMigrationFromTranslationAndDevelopmentSchemas(t *testing.T) {
	for _, version := range []int{28, 30, 31, 37, 38, 39} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			path := runtimeDatabaseFixture(t)
			if err := MigrateDatabase(path); err != nil {
				t.Fatal(err)
			}
			db, err := assetdb.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer assetdb.Close(db)
			story, err := readStory(db)
			if err != nil {
				t.Fatal(err)
			}
			if version == 28 || version == 30 || version == 31 {
				story.Rules = assets.DefaultStoryDefinitions().Rules
			} else {
				story.Rules = slices.DeleteFunc(story.Rules, func(r assets.StoryRule) bool { return r.ID == "wiki_auntie_soup_completion" })
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Where("story_key = ?", catalogMetadataID).Delete(&StoryRulesRow{}).Error; err != nil {
					return err
				}
				if err := writeStoryRules(tx, story.Rules, catalogMetadataID); err != nil {
					return err
				}
				return tx.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", version).Error
			}); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := MigrateDatabase(path); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readStory(db)
			if err != nil {
				t.Fatal(err)
			}
			found := slices.ContainsFunc(got.Rules, func(r assets.StoryRule) bool { return r.ID == "wiki_auntie_soup_completion" })
			if found != (version == 28 || version == 30 || version == 31) {
				t.Fatal("translation bridge omitted initial quest or restored development deletion")
			}
			if version >= 37 && !reflect.DeepEqual(got.Rules, story.Rules) {
				t.Fatal("name refresh changed existing quest policies")
			}
		})
	}
}

func TestWikiQuestBatchesPreserveHistoricalDeletions(t *testing.T) {
	defs := assets.DefaultStoryDefinitions()
	// A v29 administrator deliberately removed the initial soup completion.
	var batch2 []assets.StoryRule
	if err := json.Unmarshal(wikiQuestBatch2Defaults, &batch2); err != nil {
		t.Fatal(err)
	}
	defs.Rules = append(defs.Rules, batch2...)
	db := storyReadFixture(t, defs)
	for _, migrate := range []func(*gorm.DB) error{migrateRemainingWikiQuests, migrateWikiQuestCompanions, migrateWikiQuestDisguises, migrateWikiQuestActors, migrateWikiQuestConstellations, migrateRemainingWikiQuests, migrateWikiQuestCompanions, migrateWikiQuestDisguises, migrateWikiQuestActors, migrateWikiQuestConstellations} {
		if err := db.Transaction(migrate); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readStory(db)
	if err != nil || len(got.Rules) != len(defs.Rules)+86+15+13+877+22 || !reflect.DeepEqual(got.Rules[:len(defs.Rules)], defs.Rules) {
		t.Fatal("later batches altered historical policies", err)
	}
	for _, r := range got.Rules {
		if r.ID == "wiki_auntie_soup_completion" {
			t.Fatal("historical deletion restored")
		}
	}
}

func TestWikiQuestEnablingPreservesOperatorReasons(t *testing.T) {
	db := storyReadFixture(t, assets.DefaultStoryDefinitions())
	if err := db.AutoMigrate(&DisabledEventsRow{}); err != nil {
		t.Fatal(err)
	}
	want := map[uint32]string{assets.EventKey(12765, 13): "unsupported_companion_or_subject", assets.EventKey(12765, 15): "operator maintenance", assets.EventKey(12765, 99): "unsupported_companion_or_subject"}
	if err := writeDisabledEvents(db, want); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := db.Transaction(migrateEnabledWikiQuests); err != nil {
			t.Fatal(err)
		}
	}
	delete(want, assets.EventKey(12765, 13))
	got, err := readDisabledEvents(db)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("operator disable policy changed", got, err)
	}
}

func TestWikiQuestConditionMigrationPreservesEdits(t *testing.T) {
	for _, scenario := range []string{"new", "edited", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			defs := assets.DefaultStoryDefinitions()
			added := 1
			if scenario == "edited" {
				defs.Rules = append(defs.Rules, assets.StoryRule{ID: "wiki_sledge_wolf_capture_subject", Kind: assets.StoryCondition, Value: 0})
				added = 0
			}
			if scenario == "empty" {
				defs.Rules = nil
				added = 0
			}
			db := storyReadFixture(t, defs)
			for range 2 {
				if err := db.Transaction(migrateWikiQuestConditions); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readStory(db)
			if err != nil || len(got.Rules) != len(defs.Rules)+added || !reflect.DeepEqual(got.Rules[:len(defs.Rules)], defs.Rules) {
				t.Fatal("condition migration changed operator rules", err)
			}
		})
	}
}

func TestWikiQuestPoliciesPreserveEditsAndOptOut(t *testing.T) {
	defs := assets.DefaultStoryDefinitions()
	defs.Rules = append(defs.Rules, assets.StoryRule{ID: "wiki_auntie_soup_completion", Kind: assets.StoryQuestCompletion, Scope: assets.StoryScope{Maps: []uint16{65001}, Events: []uint16{99}, Branches: []byte{1}}, Mark: 999})
	db := storyReadFixture(t, defs)
	if err := db.Transaction(migrateWikiQuests); err != nil {
		t.Fatal(err)
	}
	got, err := readStory(db)
	if err != nil || len(got.Rules) != len(defs.Rules)+2 || !reflect.DeepEqual(got.Rules[:len(defs.Rules)], defs.Rules) {
		t.Fatal("existing policies changed", err)
	}
	if err := db.Transaction(migrateWikiQuests); err != nil {
		t.Fatal(err)
	}
	again, err := readStory(db)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("repeat changed policies", err)
	}
	for _, rules := range [][]assets.StoryRule{nil, {}} {
		empty := storyReadFixture(t, assets.StoryDefinitions{Rules: rules})
		if err := empty.Transaction(migrateWikiQuests); err != nil {
			t.Fatal(err)
		}
		got, err := readStory(empty)
		if err != nil || !reflect.DeepEqual(got.Rules, rules) {
			t.Fatal("empty opt-out lost", err)
		}
	}
}

func TestWikiQuestBundlesPreserveEditedOrRemovedParents(t *testing.T) {
	for _, scenario := range []string{"unchanged", "edited", "removed", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			defs := assets.DefaultStoryDefinitions()
			var err error
			defs.Rules, err = seedWikiQuests(defs.Rules, false)
			if err != nil {
				t.Fatal(err)
			}
			filtered := []assets.StoryRule{}
			for _, r := range defs.Rules {
				if r.Kind == assets.StoryQuestRewardBundle {
					continue
				}
				if r.ID == "wiki_native_completion_11149_2_20" {
					if scenario == "removed" {
						continue
					}
					if scenario == "edited" {
						r.Value++
					}
				}
				filtered = append(filtered, r)
			}
			defs.Rules = filtered
			if scenario == "empty" {
				defs.Rules = []assets.StoryRule{}
			}
			db := storyReadFixture(t, defs)
			if err := db.Transaction(migrateWikiQuestBundles); err != nil {
				t.Fatal(err)
			}
			got, err := readStory(db)
			if err != nil {
				t.Fatal(err)
			}
			added := 25
			if scenario == "edited" || scenario == "removed" {
				added = 24
			}
			if scenario == "empty" {
				added = 0
			}
			if len(got.Rules) != len(defs.Rules)+added || !reflect.DeepEqual(got.Rules[:len(defs.Rules)], defs.Rules) {
				t.Fatal("bundle migration replaced historical policy", len(got.Rules), len(defs.Rules), err)
			}
			for _, r := range got.Rules {
				if r.ID == "wiki_quest_bundle_11149_2_20" && scenario != "unchanged" {
					t.Fatal("bundle bypassed operator's parent edit/removal")
				}
			}
			if err := db.Transaction(migrateWikiQuestBundles); err != nil {
				t.Fatal(err)
			}
			again, err := readStory(db)
			if err != nil || !reflect.DeepEqual(got, again) {
				t.Fatal("bundle migration not idempotent", err)
			}
		})
	}
}
