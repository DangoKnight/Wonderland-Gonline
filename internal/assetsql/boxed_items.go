package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed boxed_items_defaults.json
var boxedItemsDefaults []byte

func defaultBoxedItems() (map[uint16]assets.BoxedItem, error) {
	var boxes []struct {
		BoxID    uint16 `json:"box_id"`
		ItemID   uint16 `json:"item_id"`
		Quantity byte   `json:"quantity"`
	}
	if err := json.Unmarshal(boxedItemsDefaults, &boxes); err != nil {
		return nil, err
	}
	values := map[uint16]assets.BoxedItem{}
	for _, b := range boxes {
		if b.BoxID == 0 || b.ItemID == 0 || b.Quantity == 0 {
			return nil, fmt.Errorf("invalid boxed item %d", b.BoxID)
		}
		if _, exists := values[b.BoxID]; exists {
			return nil, fmt.Errorf("duplicate boxed item %d", b.BoxID)
		}
		values[b.BoxID] = assets.BoxedItem{ItemID: b.ItemID, Quantity: b.Quantity}
	}
	return values, validateBoxedItems(values)
}

func validateBoxedItems(values map[uint16]assets.BoxedItem) error {
	for id, b := range values {
		if id == 0 || b.ItemID == 0 || b.Quantity == 0 {
			return fmt.Errorf("invalid boxed item %d", id)
		}
	}
	return nil
}

func migrateBoxedItems(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_boxed_items", "boxed_items", func() error {
		values, err := defaultBoxedItems()
		if err != nil {
			return err
		}
		return writeBoxedItems(tx, values)
	})
}
