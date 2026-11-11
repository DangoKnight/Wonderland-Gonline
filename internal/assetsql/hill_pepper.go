package assetsql

import (
	"encoding/json"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

const (
	hillPepperCompletionRuleID = "wiki_native_completion_12000_16_21"
	hillPepperPriorityRuleID   = "wiki_native_priority_12000_16_21"
)

// Install only this revision's missing rule. Do not replay the historical batch
// or override an administrator's priority or edited/removed completion policy.
func migrateHillPepperBranch(tx *gorm.DB) error {
	var defaults []assets.StoryRule
	if err := json.Unmarshal(wikiQuestBatch2Defaults, &defaults); err != nil {
		return err
	}
	var completion, priority *assets.StoryRule
	for i := range defaults {
		switch defaults[i].ID {
		case hillPepperCompletionRuleID:
			completion = &defaults[i]
		case hillPepperPriorityRuleID:
			priority = &defaults[i]
		}
	}
	if completion == nil || priority == nil {
		return fmt.Errorf("missing Hill Pepper branch defaults")
	}
	existing, err := readStoryRules(tx, catalogMetadataID)
	if err != nil {
		return err
	}
	parentUnchanged := false
	for _, rule := range existing {
		if rule.ID == hillPepperPriorityRuleID {
			return nil
		}
		if rule.ID == hillPepperCompletionRuleID {
			parentUnchanged = reflect.DeepEqual(rule, *completion)
		}
	}
	if !parentUnchanged {
		return nil
	}
	raw, err := json.Marshal([]assets.StoryRule{*priority})
	if err != nil {
		return err
	}
	return migrateWikiQuestRules(tx, raw)
}
