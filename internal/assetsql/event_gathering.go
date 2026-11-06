package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed event_gathering_defaults.json
var eventGatheringDefaults []byte

func defaultEventGathering() (assets.EventGatheringRules, error) {
	var values assets.EventGatheringRules
	if err := json.Unmarshal(eventGatheringDefaults, &values); err != nil {
		return values, err
	}
	return values, validateEventGathering(values)
}

func validateEventGathering(values assets.EventGatheringRules) error {
	for _, id := range values.Timers {
		if id == 0 {
			return fmt.Errorf("invalid gathering timer")
		}
	}
	for _, p := range values.Pools {
		if p.Type == 0 || p.Variant == 0 || len(p.Items) == 0 {
			return fmt.Errorf("invalid event gathering pool")
		}
		for _, id := range p.Items {
			if id == 0 {
				return fmt.Errorf("invalid gathering item")
			}
		}
	}
	return nil
}

func migrateEventGathering(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_event_gathering", "", func() error {
		values, err := defaultEventGathering()
		if err != nil {
			return err
		}
		return writeEventGathering(tx, values)
	})
}
