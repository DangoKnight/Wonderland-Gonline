package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed travel_defaults.json
var travelDefaults []byte

func defaultTravel() (assets.TravelRules, error) {
	var values assets.TravelRules
	if err := json.Unmarshal(travelDefaults, &values); err != nil {
		return values, err
	}
	return values, validateTravel(values)
}

func validateTravel(values assets.TravelRules) error {
	for id, dst := range values.ReturnScrolls {
		if id == 0 || dst.Map == 0 {
			return fmt.Errorf("invalid return scroll")
		}
	}
	for id, count := range values.Scrolls {
		if id == 0 || count < 1 || count > 3 {
			return fmt.Errorf("invalid teleport scroll")
		}
	}
	for id, dst := range values.Destinations {
		if id == 0 || dst.Map == 0 {
			return fmt.Errorf("invalid travel destination")
		}
	}
	for _, id := range values.BambooItems {
		if id == 0 {
			return fmt.Errorf("invalid Bamboo Dragonfly item")
		}
	}
	return nil
}

func migrateTravel(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_travel", "", func() error {
		values, err := defaultTravel()
		if err != nil {
			return err
		}
		return writeTravel(tx, values)
	})
}
