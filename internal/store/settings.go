package store

import (
	"context"
	"errors"
	"math"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	minimumExpRate  = 0.01
	maximumExpRate  = 1000.0
	minimumDropRate = 0.1
	maximumDropRate = 100.0
)

func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	var rows []settingRow
	if err := s.orm.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, row := range rows {
		result[row.Key] = row.Value
	}
	return result, nil
}
func (s *Store) SaveSettings(ctx context.Context, settings map[string]string) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		for k, v := range settings {
			switch k {
			case "server_name", "motd":
			case "exp_rate", "drop_rate":
				rate, err := strconv.ParseFloat(v, 64)
				minimum, maximum := minimumExpRate, maximumExpRate
				if k == "drop_rate" {
					minimum, maximum = minimumDropRate, maximumDropRate
				}
				if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < minimum || rate > maximum {
					return errors.New("invalid rate")
				}
			case "status_mode":
				if v != "auto" && v != "green" && v != "yellow" && v != "red" {
					return errors.New("invalid status mode")
				}
			case "log_level":
				if v != "debug" && v != "info" && v != "warn" && v != "error" {
					return errors.New("invalid log level")
				}
			default:
				return errors.New("unknown setting")
			}
			if len(v) > maxSettingBytes || (k == "server_name" && (len(v) == 0 || len(v) > maxServerNameBytes)) {
				return errors.New("invalid setting length")
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&settingRow{Key: k, Value: v}).Error; err != nil {
				return err
			}
		}
		return adminAudit(tx, "runtime settings update", "server")
	})
}
