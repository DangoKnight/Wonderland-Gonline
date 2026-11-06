package assetsql

import (
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"wonderland-gonline/internal/assets"
)

//go:embed npc_shop_defaults.json
var npcShopDefaults []byte

func defaultNPCShopDefaults() (map[uint16][]uint16, error) {
	var values map[uint16][]uint16
	if err := json.Unmarshal(npcShopDefaults, &values); err != nil {
		return values, err
	}
	return values, validateNPCShopDefaults(values)
}

func validateNPCShopDefaults(values map[uint16][]uint16) error {
	for service, ids := range values {
		if service < 1 || service > 3 {
			return fmt.Errorf("invalid NPC shop service")
		}
		for _, id := range ids {
			if id == 0 {
				return fmt.Errorf("invalid NPC shop item")
			}
		}
	}
	return nil
}

func migrateNPCShopDefaults(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_n_p_c_shop_defaults", "npc_shop_defaults", func() error {
		values, err := defaultNPCShopDefaults()
		if err != nil {
			return err
		}
		return writeNPCShopDefaults(tx, values)
	})
}

//go:embed npc_buy_prices_defaults.csv
var npcBuyPricesDefaults string

func defaultNPCBuyPrices(items map[uint16]assets.NativeItem) (map[uint16]uint32, error) {
	records, err := csv.NewReader(strings.NewReader(npcBuyPricesDefaults)).ReadAll()
	if err != nil {
		return nil, err
	}
	values := map[uint16]uint32{}
	for _, r := range records[1:] {
		if len(r) != 2 {
			return nil, fmt.Errorf("invalid NPC price row")
		}
		id, err := strconv.ParseUint(r[0], 10, 16)
		if err != nil {
			return nil, err
		}
		price, err := strconv.ParseUint(r[1], 10, 32)
		if err != nil {
			return nil, err
		}
		values[uint16(id)] = uint32(price)
	}
	for id, price := range nativeBuyPrices(items) {
		values[id] = price
	}
	return values, validateNPCBuyPrices(values)
}

func validateNPCBuyPrices(values map[uint16]uint32) error {
	for id := range values {
		if id == 0 {
			return fmt.Errorf("invalid NPC purchase price")
		}
	}
	return nil
}

func migrateNPCBuyPrices(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_n_p_c_buy_prices", "npc_buy_prices", func() error {
		items, err := readNativeItems(tx)
		if err != nil {
			return err
		}
		values, err := defaultNPCBuyPrices(items)
		if err != nil {
			return err
		}
		return writeNPCBuyPrices(tx, values)
	})
}

// Native buy prices are projected only by offline initialization/migration.
// Gameplay and administration continue to use the editable typed SQL dataset.
func nativeBuyPrices(items map[uint16]assets.NativeItem) map[uint16]uint32 {
	prices := make(map[uint16]uint32, len(items))
	for id, item := range items {
		if price := item.BuyPrice(); price > 0 {
			prices[id] = price
		}
	}
	return prices
}
