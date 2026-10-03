package store

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
			if k != "server_name" && k != "motd" {
				return errors.New("unknown setting")
			}
			if len(v) > maxSettingBytes || (k == "server_name" && (len(v) == 0 || len(v) > maxServerNameBytes)) {
				return errors.New("invalid setting length")
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&settingRow{Key: k, Value: v}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
