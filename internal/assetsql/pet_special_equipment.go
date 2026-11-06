package assetsql

import (
	_ "embed"
	"encoding/json"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

//go:embed pet_special_equipment_defaults.json
var petSpecialEquipmentDefaults []byte

func seedPetSpecialEquipment(existing []assets.StoryCompanionEquipment, upgrade bool) ([]assets.StoryCompanionEquipment, error) {
	var defaults []assets.StoryCompanionEquipment
	if err := json.Unmarshal(petSpecialEquipmentDefaults, &defaults); err != nil {
		return nil, err
	}
	if upgrade && len(existing) == 0 {
		return existing, nil
	} // Intentionally empty definitions stay empty.
	legacy := assets.DefaultStoryDefinitions().CompanionEquipment
	for _, definition := range defaults {
		present := false
		for _, current := range existing {
			if game.SamePet(current.Template, definition.Template) {
				present = true
				break
			}
		}
		if upgrade {
			for _, previous := range legacy {
				if game.SamePet(previous.Template, definition.Template) {
					present = true
					break
				}
			}
		}
		if !present {
			existing = append(existing, definition)
		}
	}
	return existing, nil
}

func migratePetSpecialEquipment(tx *gorm.DB) error {
	existing, err := readStoryCompanionEquipment(tx, catalogMetadataID)
	if err != nil {
		return err
	}
	seeded, err := seedPetSpecialEquipment(existing, true)
	if err != nil {
		return err
	}
	var lastOrdinal int
	if err := tx.Model(&StoryCompanionEquipmentRow{}).Where("story_key = ?", catalogMetadataID).Select("COALESCE(MAX(story_companion_equipment_ordinal), -1)").Scan(&lastOrdinal).Error; err != nil {
		return err
	}
	for index := len(existing); index < len(seeded); index++ {
		lastOrdinal++
		ordinal := lastOrdinal
		definition := seeded[index]
		row := StoryCompanionEquipmentRow{StoryKey: catalogMetadataID, StoryCompanionEquipmentOrdinal: ordinal, ValueAllowMissing: definition.AllowMissing, ValueTemplate: definition.Template, ValueEquipmentPresent: definition.Equipment != nil}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := writeStoryCompanionEquipmentEquipment(tx, definition.Equipment, catalogMetadataID, ordinal); err != nil {
			return err
		}
	}
	return nil
}
