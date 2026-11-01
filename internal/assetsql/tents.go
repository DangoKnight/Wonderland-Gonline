package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"reflect"
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

// Update only the exact previously shipped shared defaults. Owned tents and
// administrator-authored definitions are deliberately left intact.
func migrateTentDefaults(tx *gorm.DB) error {
	current, err := readTents(tx)
	if err != nil {
		return err
	}
	old := assets.TentRules{SpawnX: 460, SpawnY: 700, Floor: 39062, Wallpaper: 39064, Furniture: []assets.TentFurnitureDefault{{ItemID: 38027, X: 43, Y: 42}, {ItemID: 38049, X: 45, Y: 42}}}
	if !reflect.DeepEqual(current, old) {
		return nil
	}
	next, err := defaultTents()
	if err != nil {
		return err
	}
	if err = tx.Where("tents_key = ?", catalogMetadataID).Delete(&TentsFurnitureRow{}).Error; err != nil {
		return err
	}
	if err = tx.Where("tents_key = ?", catalogMetadataID).Delete(&TentsRow{}).Error; err != nil {
		return err
	}
	return writeTents(tx, next)
}
