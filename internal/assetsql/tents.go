package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"wonderland-gonline/internal/assets"
)

//go:embed tent_defaults.json
var tentDefaults []byte

func defaultTents() (assets.TentRules, error) {
	var r assets.TentRules
	err := json.Unmarshal(tentDefaults, &r)
	return r, err
}
func validateTents(r assets.TentRules) error {
	for _, item := range r.Furniture {
		if item.ItemID == 0 || item.Floor != 0 {
			return fmt.Errorf("invalid default tent furniture")
		}
	}
	return nil
}
