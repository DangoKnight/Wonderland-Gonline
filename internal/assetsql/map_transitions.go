package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed map_transitions_defaults.json
var mapTransitionsDefaults []byte

func defaultMapTransitions() ([]assets.MapTransition, error) {
	var values []assets.MapTransition
	if err := json.Unmarshal(mapTransitionsDefaults, &values); err != nil {
		return values, err
	}
	return values, validateMapTransitions(values)
}

func validateMapTransitions(values []assets.MapTransition) error {
	seenTransitions := map[[2]uint16]bool{}
	for _, r := range values {
		key := [2]uint16{r.FromMap, r.Entry}
		if r.FromMap == 0 || r.Entry == 0 || r.Destination.Map == 0 || seenTransitions[key] || len(r.Items) > 0 && r.Quest == 0 {
			return fmt.Errorf("invalid map transition")
		}
		seenTransitions[key] = true
		for _, item := range r.Items {
			if item.ItemID == 0 || item.Quantity == 0 {
				return fmt.Errorf("invalid transition reward")
			}
		}
	}
	return nil
}

func migrateMapTransitions(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_map_transitions", "map_transitions", func() error {
		values, err := defaultMapTransitions()
		if err != nil {
			return err
		}
		return writeMapTransitions(tx, values)
	})
}
