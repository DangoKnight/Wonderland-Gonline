package assetsql

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"wonderland-gonline/internal/game"
)

// Offline seed only. Runtime recruitment reads the structured SQL table.
//
//go:embed pet_initial_stats_defaults.json
var petInitialStatsDefaults []byte

func defaultPetInitialStats() (map[uint32]game.PetInitialStats, error) {
	values := map[uint32]game.PetInitialStats{}
	if err := json.Unmarshal(petInitialStatsDefaults, &values); err != nil {
		return nil, err
	}
	return values, validatePetInitialStats(values)
}

func validatePetInitialStats(values map[uint32]game.PetInitialStats) error {
	for id, v := range values {
		lo, hi := v.Minimum, v.Maximum
		if id == 0 || v.Source == "" || lo.Strength > hi.Strength || lo.Constitution > hi.Constitution || lo.Intelligence > hi.Intelligence || lo.Wisdom > hi.Wisdom || lo.Agility > hi.Agility {
			return fmt.Errorf("invalid pet initial stats %d", id)
		}
		sum := func(a game.Attributes) uint32 {
			return uint32(a.Strength) + uint32(a.Constitution) + uint32(a.Intelligence) + uint32(a.Wisdom) + uint32(a.Agility)
		}
		if v.TotalPoints < sum(lo) || v.TotalPoints > sum(hi) {
			return fmt.Errorf("pet initial stat total outside ranges for %d", id)
		}
	}
	return nil
}

func migratePetInitialStats(tx *gorm.DB) error {
	return migrateCatalogDataset(tx, "catalog_pet_initial_stats", "pet_initial_stats", func() error {
		values, err := defaultPetInitialStats()
		if err != nil {
			return err
		}
		return writePetInitialStats(tx, values)
	})
}
