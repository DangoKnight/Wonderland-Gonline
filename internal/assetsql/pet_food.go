package assetsql

import (
	"fmt"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

const iceSnowflakeItemID = 27025
const darkSnowGirlTemplateID = 31235

// Initialization only: runtime food compatibility is owned by structured SQL.
func defaultPetFoods(items map[uint16]assets.NativeItem) map[uint16]assets.PetFood {
	foods := map[uint16]assets.PetFood{}
	for id, item := range items {
		if target := item.PetFoodTarget(); target != 0 {
			foods[id] = assets.PetFood{Targets: []uint32{target}}
		}
	}
	if food, ok := foods[iceSnowflakeItemID]; ok {
		food.Targets = append(food.Targets, darkSnowGirlTemplateID)
		foods[iceSnowflakeItemID] = food
	}
	return foods
}

func validatePetFoods(foods map[uint16]assets.PetFood) error {
	for id, food := range foods {
		if id == 0 || len(food.Targets) == 0 {
			return fmt.Errorf("invalid pet food %d", id)
		}
		seen := map[uint32]bool{}
		for _, target := range food.Targets {
			if target == 0 || seen[target] {
				return fmt.Errorf("invalid pet food target %d", id)
			}
			seen[target] = true
		}
	}
	return nil
}

func migratePetFoods(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_pet_foods", "pet_foods", func() error {
		items, err := readNativeItems(tx)
		if err != nil {
			return err
		}
		return writePetFoods(tx, defaultPetFoods(items))
	})
}
