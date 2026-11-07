package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// Offline initialization inputs; gameplay reads catalog_mall only.
//
//go:embed mall_defaults.json
var mallDefaults []byte

// Exact previous authored import, retained to distinguish defaults from edits.
//
//go:embed mall_legacy_defaults.json
var mallLegacyDefaults []byte

// Exact v59 screenshot catalog, preserved as an independently authored baseline.
//
//go:embed mall_screenshot_defaults.json
var mallScreenshotDefaults []byte

func defaultMall() ([]assets.MallItem, error) {
	var entries []assets.MallItem
	if err := json.Unmarshal(mallDefaults, &entries); err != nil {
		return nil, err
	}
	seen := map[[4]int]bool{}
	for _, entry := range entries {
		key := [4]int{entry.Bonus, int(entry.ID), entry.CategoryID, entry.Order}
		if entry.ID == 0 || entry.Count == 0 || entry.Cost <= 0 || entry.OriginalPrice > math.MaxUint16 || entry.Discount <= 0 || entry.Discount > 100 || entry.CategoryID < 1 || entry.CategoryID > 5 || entry.Order <= 0 || entry.Order > math.MaxUint16 || (entry.Bonus != 0 && entry.Bonus != 1) || seen[key] || int(math.RoundToEven(float64(entry.OriginalPrice)*float64(entry.Discount)/100)) != entry.Cost {
			return nil, fmt.Errorf("invalid mall default %+v", entry)
		}
		seen[key] = true
	}
	return entries, nil
}

func updateDefaultMall(current []assets.MallItem) ([]assets.MallItem, error) {
	var previous []assets.MallItem
	if err := json.Unmarshal(mallLegacyDefaults, &previous); err != nil {
		return nil, err
	}
	// An intentionally empty catalog, changed price, added/deleted row or reordered
	// catalog is an administrator-owned configuration; leave the entire set intact.
	var screenshot []assets.MallItem
	if err := json.Unmarshal(mallScreenshotDefaults, &screenshot); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(current, previous) && !reflect.DeepEqual(current, screenshot) {
		return current, nil
	}
	return defaultMall()
}

func migrateMallDefaults(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&MallRow{}) {
		return nil
	}
	current, err := readMall(tx)
	if err != nil {
		return err
	}
	updated, err := updateDefaultMall(current)
	if err != nil || reflect.DeepEqual(current, updated) {
		return err
	}
	if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&MallRow{}).Error; err != nil {
		return err
	}
	return writeMall(tx, updated)
}
