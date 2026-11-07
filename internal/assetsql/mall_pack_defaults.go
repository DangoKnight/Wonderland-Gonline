package assetsql

import (
	_ "embed"
	"encoding/json"
	"reflect"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// Offline authored pools: official EXP percentages where contents/effects match;
// otherwise near-equal 10,000-slot weights, as requested by the administrator.
//
//go:embed mall_pack_defaults.json
var mallPackDefaults []byte

// Exact old authored pools. Only these unedited values may be replaced.
//
//go:embed mall_pack_previous.json
var mallPackPrevious []byte

// Its published featured prizes account for only 1.2%; unnamed props cannot
// be invented or omitted while presenting a complete purchasable reward pool.
const incompleteWinterClothBagID uint16 = 56276

func migrateMallPackDefaults(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&GachaPacksRow{}) {
		return nil
	}
	current, err := readGachaPacks(tx)
	if err != nil {
		return err
	}
	if len(current) == 0 {
		return nil
	} // Intentionally empty administrator catalog.
	native, err := readNativeItems(tx)
	if err != nil {
		return err
	}
	items := map[uint16]game.ItemDefinition{}
	for id, n := range native {
		items[id] = n.Definition
	}
	defaults, _, err := assets.ParseCompatibleGachaPacks(mallPackDefaults, items)
	if err != nil {
		return err
	}
	var prior []assets.GachaPack
	if err := json.Unmarshal(mallPackPrevious, &prior); err != nil {
		return err
	}
	previous := map[uint16]assets.GachaPack{}
	for _, p := range prior {
		previous[p.ID] = p
	}
	unavailable, err := readUnavailableGachaPacks(tx)
	if err != nil {
		return err
	}
	for id, pack := range defaults {
		old, exists := current[id]
		if _, blocked := unavailable[id]; blocked {
			continue
		}
		if exists && !reflect.DeepEqual(old, previous[id]) {
			continue
		}
		if exists {
			if err := tx.Where("gacha_packs_key = ?", id).Delete(&GachaPacksRewardsRow{}).Error; err != nil {
				return err
			}
			if err := tx.Where("gacha_packs_key = ?", id).Delete(&GachaPacksRow{}).Error; err != nil {
				return err
			}
		}
		if err := writeGachaPacks(tx, map[uint16]assets.GachaPack{id: pack}); err != nil {
			return err
		}
	}
	if _, known := items[incompleteWinterClothBagID]; !known {
		return nil
	}
	if _, configured := current[incompleteWinterClothBagID]; !configured {
		if _, blocked := unavailable[incompleteWinterClothBagID]; !blocked {
			return writeUnavailableGachaPacks(tx, map[uint16]assets.GachaPackIssue{incompleteWinterClothBagID: {ID: incompleteWinterClothBagID}})
		}
	}
	return nil
}
