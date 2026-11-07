package assetsql

import (
	_ "embed"
	"encoding/json"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

//go:embed item_cards_defaults.json
var itemCardsDefaults []byte

func defaultItemCards() (map[uint16]assets.ItemCard, error) {
	var rows []struct {
		ID uint16 `json:"card_id"`
		assets.ItemCard
	}
	if err := json.Unmarshal(itemCardsDefaults, &rows); err != nil {
		return nil, err
	}
	result := make(map[uint16]assets.ItemCard, len(rows))
	for _, row := range rows {
		result[row.ID] = row.ItemCard
	}
	return result, nil
}

func migrateItemCards(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_item_cards", "item_cards", func() error {
		values, err := defaultItemCards()
		if err != nil {
			return err
		}
		return writeItemCards(tx, values)
	})
}
