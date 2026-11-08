package assetsql

import (
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// Add only the new policy, preserving all existing rules and administrator edits.
func migrateAccountGroundStory(tx *gorm.DB) error {
	// Partial catalogs used by older installations may not own Story yet.
	// Leave absent datasets absent, just as intentionally empty rules stay empty.
	if !tx.Migrator().HasTable(&StoryRow{}) {
		return nil
	}
	defs, err := readStory(tx)
	if err != nil {
		return err
	}
	if len(defs.Rules) == 0 {
		return nil
	}
	changed := false
	for _, seed := range assets.DefaultStoryDefinitions().Rules {
		if seed.Kind != assets.StoryAccountGroundOnce {
			continue
		}
		found := false
		for _, rule := range defs.Rules {
			if rule.ID == seed.ID {
				found = true
				break
			}
		}
		if !found {
			defs.Rules = append(defs.Rules, seed)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := tx.Where("story_key = ?", catalogMetadataID).Delete(&StoryRulesRow{}).Error; err != nil {
		return err
	}
	return writeStoryRules(tx, defs.Rules, catalogMetadataID)
}
