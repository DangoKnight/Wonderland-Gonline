package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"
	"wonderland-gonline/internal/assets"

	"gorm.io/gorm"
)

//go:embed native_quest_revision_v43.json
var nativeQuestRevisionV43 []byte

//go:embed native_quest_revision_v44.json
var nativeQuestRevisionV44 []byte

//go:embed native_quest_revision_v45.json
var nativeQuestRevisionV45 []byte

//go:embed native_quest_revision_v46.json
var nativeQuestRevisionV46 []byte

//go:embed wiki_quest_defaults.json
var wikiQuestDefaults []byte

//go:embed wiki_quest_batch2_defaults.json
var wikiQuestBatch2Defaults []byte

//go:embed wiki_quest_batch3_defaults.json
var wikiQuestBatch3Defaults []byte

//go:embed wiki_quest_companion_defaults.json
var wikiQuestCompanionDefaults []byte

//go:embed wiki_quest_disguise_defaults.json
var wikiQuestDisguiseDefaults []byte

//go:embed wiki_quest_actor_defaults.json
var wikiQuestActorDefaults []byte

//go:embed wiki_quest_constellation_defaults.json
var wikiQuestConstellationDefaults []byte

//go:embed wiki_quest_bundle_defaults.json
var wikiQuestBundleDefaults []byte

//go:embed wiki_quest_bundle_parents.json
var wikiQuestBundleParents []byte

//go:embed wiki_quest_condition_defaults.json
var wikiQuestConditionDefaults []byte

func seedWikiQuestBundles(existing []assets.StoryRule, upgrade bool) ([]assets.StoryRule, error) {
	if upgrade && len(existing) == 0 {
		return existing, nil
	}
	var defaults []assets.StoryRule
	var parents map[string]string
	if err := json.Unmarshal(wikiQuestBundleDefaults, &defaults); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(wikiQuestBundleParents, &parents); err != nil {
		return nil, err
	}
	expected := map[string]assets.StoryRule{}
	for _, raw := range [][]byte{wikiQuestDefaults, wikiQuestBatch2Defaults, wikiQuestBatch3Defaults} {
		var batch []assets.StoryRule
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, err
		}
		for _, r := range batch {
			expected[r.ID] = r
		}
	}
	current := map[string]assets.StoryRule{}
	for _, r := range existing {
		current[r.ID] = r
	}
	for _, r := range defaults {
		if _, present := current[r.ID]; present {
			continue
		}
		if parent := parents[r.ID]; parent != "" {
			want, known := expected[parent]
			if !known {
				return nil, fmt.Errorf("quest bundle %s has unknown parent %s", r.ID, parent)
			}
			if actual, present := current[parent]; !present || !reflect.DeepEqual(actual, want) {
				continue
			}
		}
		existing = append(existing, r)
	}
	return existing, nil
}

//go:embed wiki_quest_enabled_events.json
var wikiQuestEnabledEvents []byte

type enabledWikiQuestEvent struct {
	Map            uint16 `json:"map"`
	Event          uint16 `json:"event"`
	ObsoleteReason string `json:"obsolete_reason"`
}

func enabledWikiQuestEvents() ([]enabledWikiQuestEvent, error) {
	var events []enabledWikiQuestEvent
	err := json.Unmarshal(wikiQuestEnabledEvents, &events)
	return events, err
}

func initializeEnabledWikiQuests(c *assets.Catalog) error {
	events, err := enabledWikiQuestEvents()
	if err != nil {
		return err
	}
	for _, event := range events {
		key := assets.EventKey(event.Map, event.Event)
		if c.DisabledEvents[key] == event.ObsoleteReason {
			delete(c.DisabledEvents, key)
		}
	}
	return nil
}

func migrateEnabledWikiQuests(tx *gorm.DB) error {
	events, err := enabledWikiQuestEvents()
	if err != nil {
		return err
	}
	for _, event := range events {
		// A different operator-authored reason must remain disabled.
		if err := tx.Where("disabled_events_key = ? AND value = ?", assets.EventKey(event.Map, event.Event), event.ObsoleteReason).Delete(&DisabledEventsRow{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedWikiQuestRules(existing []assets.StoryRule, raw []byte, upgrade bool) ([]assets.StoryRule, error) {
	if upgrade && len(existing) == 0 {
		return existing, nil
	}
	var defaults []assets.StoryRule
	if err := json.Unmarshal(raw, &defaults); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, rule := range existing {
		seen[rule.ID] = true
	}
	for _, rule := range defaults {
		if !seen[rule.ID] {
			existing = append(existing, rule)
		}
	}
	return existing, nil
}

func seedWikiQuests(existing []assets.StoryRule, upgrade bool) ([]assets.StoryRule, error) {
	seeded, err := seedWikiQuestRules(existing, wikiQuestDefaults, upgrade)
	if err != nil {
		return nil, err
	}
	seeded, err = seedWikiQuestRules(seeded, wikiQuestBatch2Defaults, upgrade)
	if err != nil {
		return nil, err
	}
	seeded, err = seedWikiQuestRules(seeded, wikiQuestBatch3Defaults, upgrade)
	if err != nil {
		return nil, err
	}
	seeded, err = seedWikiQuestRules(seeded, wikiQuestCompanionDefaults, upgrade)
	if err != nil {
		return nil, err
	}
	seeded, err = seedWikiQuestRules(seeded, wikiQuestDisguiseDefaults, upgrade)
	if err != nil {
		return nil, err
	}
	seeded, err = seedWikiQuestRules(seeded, wikiQuestActorDefaults, upgrade)
	if err != nil {
		return nil, err
	}
	seeded, err = seedWikiQuestRules(seeded, wikiQuestConstellationDefaults, upgrade)
	if err != nil {
		return nil, err
	}
	seeded, err = seedWikiQuestBundles(seeded, upgrade)
	if err != nil {
		return nil, err
	}
	return seedWikiQuestRules(seeded, wikiQuestConditionDefaults, upgrade)
}

func migrateWikiQuestConditions(tx *gorm.DB) error {
	return migrateWikiQuestRules(tx, wikiQuestConditionDefaults)
}

func migrateWikiQuests(tx *gorm.DB) error {
	return migrateWikiQuestRules(tx, wikiQuestDefaults)
}

func migrateAdditionalWikiQuests(tx *gorm.DB) error {
	return migrateWikiQuestRules(tx, wikiQuestBatch2Defaults)
}

func migrateRemainingWikiQuests(tx *gorm.DB) error {
	return migrateWikiQuestRules(tx, wikiQuestBatch3Defaults)
}

func migrateWikiQuestCompanions(tx *gorm.DB) error {
	return migrateWikiQuestRules(tx, wikiQuestCompanionDefaults)
}

func migrateWikiQuestDisguises(tx *gorm.DB) error {
	return migrateWikiQuestRules(tx, wikiQuestDisguiseDefaults)
}

func migrateWikiQuestActors(tx *gorm.DB) error {
	return migrateWikiQuestRules(tx, wikiQuestActorDefaults)
}

func migrateWikiQuestConstellations(tx *gorm.DB) error {
	return migrateWikiQuestRules(tx, wikiQuestConstellationDefaults)
}

func migrateWikiQuestBundles(tx *gorm.DB) error {
	existing, err := readStoryRules(tx, catalogMetadataID)
	if err != nil {
		return err
	}
	seeded, err := seedWikiQuestBundles(existing, true)
	if err != nil || len(seeded) == len(existing) {
		return err
	}
	if err := tx.Where("story_key = ?", catalogMetadataID).Delete(&StoryRulesRow{}).Error; err != nil {
		return err
	}
	return writeStoryRules(tx, seeded, catalogMetadataID)
}

func migrateWikiQuestRules(tx *gorm.DB, raw []byte) error {
	existing, err := readStoryRules(tx, catalogMetadataID)
	if err != nil {
		return err
	}
	seeded, err := seedWikiQuestRules(existing, raw, true)
	if err != nil || len(seeded) == len(existing) {
		return err
	}
	// Rewrite this owned collection transactionally, retaining every existing
	// value and ordinal. Child rows follow the existing cascade relationships.
	if err := tx.Where("story_key = ?", catalogMetadataID).Delete(&StoryRulesRow{}).Error; err != nil {
		return err
	}
	return writeStoryRules(tx, seeded, catalogMetadataID)
}
