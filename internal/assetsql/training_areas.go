package assetsql

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"wonderland-gonline/internal/assets"
)

// Authored from the stored Training Island map family and administrator rules
// confirmed on 2026-11-09. Runtime policy is read from the structured map rows.
//
//go:embed training_areas_defaults.csv
var trainingAreaDefaults string

func defaultTrainingAreas() (map[uint16]assets.Map, error) {
	rows, err := csv.NewReader(strings.NewReader(trainingAreaDefaults)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 || strings.Join(rows[0], ",") != "map_id,name,round_recovery_hp,round_recovery_sp,victory_exp" {
		return nil, fmt.Errorf("invalid training area defaults header")
	}
	result := map[uint16]assets.Map{}
	for _, row := range rows[1:] {
		if len(row) != 5 {
			return nil, fmt.Errorf("invalid training area row")
		}
		var numbers [4]uint16
		for i, at := range []int{0, 2, 3, 4} {
			n, err := strconv.ParseUint(row[at], 10, 16)
			if err != nil {
				return nil, err
			}
			numbers[i] = uint16(n)
		}
		id := numbers[0]
		if _, exists := result[id]; id == 0 || exists {
			return nil, fmt.Errorf("invalid training area ID %d", id)
		}
		result[id] = assets.Map{ID: id, BattleRoundRecoveryHP: numbers[1], BattleRoundRecoverySP: numbers[2], BattleVictoryEXP: numbers[3]}
	}
	return result, nil
}

func addMapBattleColumns(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&MapsRow{}) {
		return nil
	}
	for _, field := range []string{"ValueBattleRoundRecoveryHP", "ValueBattleRoundRecoverySP", "ValueBattleVictoryEXP"} {
		if !tx.Migrator().HasColumn(&MapsRow{}, field) {
			if err := tx.Migrator().AddColumn(&MapsRow{}, field); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedTrainingAreas(c *assets.Catalog) error {
	values, err := defaultTrainingAreas()
	if err != nil {
		return err
	}
	for id, value := range values {
		if row, exists := c.Maps[id]; exists {
			row.BattleRoundRecoveryHP = value.BattleRoundRecoveryHP
			row.BattleRoundRecoverySP = value.BattleRoundRecoverySP
			row.BattleVictoryEXP = value.BattleVictoryEXP
			c.Maps[id] = row
		}
	}
	return nil
}

func migrateTrainingAreas(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&MapsRow{}) {
		return nil
	}
	values, err := defaultTrainingAreas()
	if err != nil {
		return err
	}
	for id, value := range values {
		if err := tx.Model(&MapsRow{}).Where("maps_key = ?", id).Updates(map[string]any{
			"value_battle_round_recovery_hp": value.BattleRoundRecoveryHP,
			"value_battle_round_recovery_sp": value.BattleRoundRecoverySP,
			"value_battle_victory_exp":       value.BattleVictoryEXP,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
