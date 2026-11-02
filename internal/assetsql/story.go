package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// v15 seeds story policies exactly once. Existing edits and deliberate empty
// definitions remain authoritative, including on a repeated offline migration.
func migrateStory(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_story", "", func() error {
		defs := assets.DefaultStoryDefinitions()
		if err := assets.ValidateStoryDefinitions(defs); err != nil {
			return err
		}
		return writeStory(tx, defs)
	})
}

// v16 adds owned mark lists to story rules and seeds the new rebirth checkpoint.
// Presence of the column records initialization: repeats must retain removed or
// edited rules. Empty rule collections are deliberate opt-outs and stay empty.
func migrateRebornStoryCheckpoints(tx *gorm.DB) error {
	added := !tx.Migrator().HasColumn(&StoryRulesRow{}, "ValueMarksPresent")
	if added {
		if err := tx.Migrator().AddColumn(&StoryRulesRow{}, "ValueMarksPresent"); err != nil {
			return err
		}
	}
	if !tx.Migrator().HasTable(&StoryRulesMarksRow{}) {
		if err := tx.Migrator().CreateTable(&StoryRulesMarksRow{}); err != nil {
			return err
		}
	}
	if !added {
		return nil
	}
	defs, err := readStory(tx)
	if err != nil {
		return err
	}
	if len(defs.Rules) == 0 {
		return nil
	}
	ids := make(map[string]bool, len(defs.Rules))
	for _, rule := range defs.Rules {
		ids[rule.ID] = true
	}
	changed := false
	for _, rule := range assets.DefaultStoryDefinitions().Rules {
		if rule.Kind == assets.StoryRebornCheckpoint && !ids[rule.ID] {
			defs.Rules = append(defs.Rules, rule)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	// Rewrite only this owned rule collection, keeping its existing values and
	// ordinals. The migration runner wraps column/table/data changes atomically.
	if err := tx.Where("story_key = ?", catalogMetadataID).Delete(&StoryRulesRow{}).Error; err != nil {
		return err
	}
	return writeStoryRules(tx, defs.Rules, catalogMetadataID)
}
