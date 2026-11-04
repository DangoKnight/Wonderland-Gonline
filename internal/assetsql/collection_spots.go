package assetsql

import (
	_ "embed"
	"encoding/json"
	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

//go:embed collection_spots_defaults.json
var collectionSpotDefaults []byte

// Offline initialization preserves source records even when the importing
// catalog lacks a map/item. Zero native IDs explicitly mark unresolved links.
func defaultCollectionSpots(c *assets.Catalog) (map[uint16]assets.CollectionSpot, error) {
	var spots map[uint16]assets.CollectionSpot
	if err := json.Unmarshal(collectionSpotDefaults, &spots); err != nil {
		return nil, err
	}
	for id, spot := range spots {
		var candidates []uint16
		for _, mid := range spot.MapCandidates {
			if _, ok := c.Maps[mid]; ok {
				candidates = append(candidates, mid)
			}
		}
		spot.MapCandidates = candidates
		if _, ok := c.Maps[spot.MapID]; !ok {
			spot.MapID = 0
		}
		for i, reward := range spot.Rewards {
			if _, ok := c.Items[reward.ItemID]; !ok {
				spot.Rewards[i].ItemID = 0
			}
		}
		spots[id] = spot
	}
	return spots, assets.ValidateCollectionSpots(spots, c.Items, c.Maps)
}

func migrateCollectionSpots(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_collection_spots", "collection_spots", func() error {
		items, err := readNativeItems(tx)
		if err != nil {
			return err
		}
		// Only map membership is needed. Avoid loading every event, branch and
		// operation just to seed this small definition dataset.
		var rows []MapsRow
		if err := tx.Select("maps_key").Find(&rows).Error; err != nil {
			return err
		}
		maps := make(map[uint16]assets.Map, len(rows))
		for _, row := range rows {
			maps[row.MapsKey] = assets.Map{ID: row.MapsKey}
		}
		c := &assets.Catalog{NativeItems: items, Maps: maps, Items: make(map[uint16]game.ItemDefinition, len(items))}
		for id, item := range items {
			c.Items[id] = item.Definition
		}
		spots, err := defaultCollectionSpots(c)
		if err != nil {
			return err
		}
		return writeCollectionSpots(tx, spots)
	})
}
