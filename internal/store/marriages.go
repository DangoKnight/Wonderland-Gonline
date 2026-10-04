package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
	"wonderland-go/internal/game"
)

var ErrAlreadyMarried = errors.New("one of these characters is already married")

func (s *Store) Marriage(ctx context.Context, id uint32) (*AdminMarriage, error) {
	var row AdminMarriage
	err := s.orm.WithContext(ctx).Where("character1 = ? OR character2 = ?", id, id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

// Marry commits the ceremony fee, both rings and relationship atomically.
func (s *Store) Marry(ctx context.Context, refs [2]CharacterRef, minimumLevel uint16, fee uint32, rings [2]uint16, items map[uint16]game.ItemDefinition, reservations ...game.Character) ([2]game.Character, [2][]game.Addition, error) {
	var chars [2]game.Character
	var adds [2][]game.Addition
	if refs[0].ID == refs[1].ID {
		return chars, adds, errors.New("distinct partners are required")
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		for i, ref := range refs {
			var count int64
			if err := tx.Model(&AdminMarriage{}).Where("character1 = ? OR character2 = ?", ref.ID, ref.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrAlreadyMarried
			}
			var err error
			chars[i], err = loadCharacter(tx, ref)
			if err != nil {
				return err
			}
			if err = preserveItemReservations(&chars[i], reservations); err != nil {
				return err
			}
			if uint16(chars[i].Level) < minimumLevel {
				return errors.New("marriage level requirement not met")
			}
			if rings[i] != 0 {
				item, known := items[rings[i]]
				if !known {
					return errors.New("wedding ring definition is unavailable")
				}
				adds[i], err = chars[i].Bag.Grant(game.Item{ID: rings[i]}, 1, item.StackLimit())
				if err != nil {
					return err
				}
			}
		}
		if fee > chars[0].Gold {
			return game.ErrTradeChanged
		}
		chars[0].Gold -= fee
		for i, ref := range refs {
			if err := saveCharacter(tx, ref, chars[i]); err != nil {
				return err
			}
		}
		return tx.Create(&AdminMarriage{Character1: refs[0].ID, Character2: refs[1].ID, At: time.Now().UTC().Format(time.RFC3339)}).Error
	})
	return chars, adds, err
}
func (s *Store) Divorce(ctx context.Context, ref CharacterRef) (uint32, error) {
	var partner uint32
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := loadCharacter(tx, ref); err != nil {
			return err
		}
		var row AdminMarriage
		if err := tx.Where("character1 = ? OR character2 = ?", ref.ID, ref.ID).Take(&row).Error; err != nil {
			return err
		}
		partner = row.Character1
		if partner == ref.ID {
			partner = row.Character2
		}
		return tx.Delete(&row).Error
	})
	return partner, err
}
