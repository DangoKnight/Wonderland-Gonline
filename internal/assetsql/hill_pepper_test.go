package assetsql

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
)

func TestHillPepperMigrationPreservesAuthoredAndAdministratorRules(t *testing.T) {
	var defaults []assets.StoryRule
	if err := json.Unmarshal(wikiQuestBatch2Defaults, &defaults); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"missing priority", "edited priority", "edited completion", "removed completion", "empty opt-out"} {
		t.Run(scenario, func(t *testing.T) {
			var rules []assets.StoryRule
			for _, rule := range defaults {
				if rule.ID == hillPepperPriorityRuleID {
					if scenario != "edited priority" {
						continue
					}
					rule.Value++
				}
				if rule.ID == hillPepperCompletionRuleID {
					if scenario == "removed completion" {
						continue
					}
					if scenario == "edited completion" {
						rule.Value++
					}
				}
				rules = append(rules, rule)
			}
			if scenario == "empty opt-out" {
				rules = []assets.StoryRule{}
			}
			db := storyReadFixture(t, assets.StoryDefinitions{Rules: rules})
			if err := db.Transaction(migrateHillPepperBranch); err != nil {
				t.Fatal(err)
			}
			got, err := readStoryRules(db, catalogMetadataID)
			if err != nil {
				t.Fatal(err)
			}
			want := rules
			if scenario == "missing priority" {
				for _, rule := range defaults {
					if rule.ID == hillPepperPriorityRuleID {
						want = append(append([]assets.StoryRule(nil), rules...), rule)
					}
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("migration did not preserve policies or install the expected priority")
			}
			if err := db.Transaction(migrateHillPepperBranch); err != nil {
				t.Fatal(err)
			}
			again, err := readStoryRules(db, catalogMetadataID)
			if err != nil || !reflect.DeepEqual(got, again) {
				t.Fatal("migration is not idempotent", err)
			}
		})
	}
}

func TestHillPepperUpgradeFromSchema70(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.db")
	if _, err := BuildRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(db)
	existing, err := readStoryRules(db, catalogMetadataID)
	if err != nil {
		t.Fatal(err)
	}
	previous := make([]assets.StoryRule, 0, len(existing))
	for _, rule := range existing {
		if rule.ID != hillPepperPriorityRuleID {
			previous = append(previous, rule)
		}
	}
	if len(previous) != len(existing)-1 {
		t.Fatal("repository seed must contain exactly one Hill Pepper priority")
	}
	// Recreate the preceding schema while retaining all native data. Include an
	// unrelated administrator edit to prove this content migration is narrow.
	previous[0].ID = "administrator_unchanged_policy"
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("story_key = ?", catalogMetadataID).Delete(&StoryRulesRow{}).Error; err != nil {
			return err
		}
		if err := writeStoryRules(tx, previous, catalogMetadataID); err != nil {
			return err
		}
		return tx.Model(&catalogSchema{}).Where("id = ?", catalogMetadataID).Update("version", trainingTicketDurationSchemaVersion).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	got, err := readStoryRules(db, catalogMetadataID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(previous)+1 || !reflect.DeepEqual(got[:len(previous)], previous) || got[len(previous)].ID != hillPepperPriorityRuleID {
		t.Fatal("schema upgrade changed unrelated policies or omitted the priority")
	}
	var schema catalogSchema
	if err := db.First(&schema, catalogMetadataID).Error; err != nil || schema.Version != catalogSchemaVersion {
		t.Fatal(schema, err)
	}
	if err := db.Transaction(migrateCatalog); err != nil {
		t.Fatal(err)
	}
	again, err := readStoryRules(db, catalogMetadataID)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("repeated schema migration changed policies", err)
	}
}
