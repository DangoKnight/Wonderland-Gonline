package store

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"wonderland-gonline/internal/game"
)

func (s *Store) Characters(ctx context.Context, account uint32) ([]game.Character, error) {
	var result []game.Character
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var rows []characterRow
		if err := tx.Omit("state").Where(map[string]any{"account_id": account}).Order(clause.OrderByColumn{Column: clause.Column{Name: "slot"}}).Find(&rows).Error; err != nil {
			return err
		}
		var err error
		result, err = decodeCharacters(tx, rows)
		return err
	})
	return result, err
}

func (s *Store) CreateCharacter(ctx context.Context, a Account, c game.Character) error {
	return s.CreateCharacterWithCode(ctx, a, c, "")
}
func (s *Store) CreateCharacterWithCode(ctx context.Context, a Account, c game.Character, code string) error {
	if c.ID != a.CharacterID(c.Slot) {
		return errors.New("character does not belong to account")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if code != "" && (len(code) < DeletionCodeMinBytes || len(code) > DeletionCodeMaxBytes) {
		return errors.New("invalid deletion code")
	}
	hash := ""
	if code != "" {
		var err error
		hash, err = hashPassword(code)
		if err != nil {
			return err
		}
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var owner accountRow
		if err := tx.Select("banned").Where(map[string]any{"id": a.ID}).Take(&owner).Error; err != nil {
			return err
		}
		if owner.Banned {
			return ErrCredentials
		}
		if err := tx.Create(&characterRow{ID: c.ID, AccountID: a.ID, Slot: c.Slot, Name: c.Name, State: []byte{}}).Error; err != nil {
			return err
		}
		if err := writeCharacterState(tx, c); err != nil {
			return err
		}
		if hash != "" {
			return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "account_id"}}, DoNothing: true}).Create(&securityRow{AccountID: a.ID, DeletionHash: hash}).Error
		}
		return nil
	})
}
func (s *Store) CharacterNameAvailable(ctx context.Context, name string) (bool, error) {
	if err := game.ValidateCharacterName(name); err != nil {
		return false, err
	}
	var count int64
	err := s.orm.WithContext(ctx).Model(&characterRow{}).Where(map[string]any{"name": name}).Count(&count).Error
	return count == 0, err
}
func (s *Store) HasDeletionCode(ctx context.Context, account uint32) (bool, error) {
	var count int64
	err := s.orm.WithContext(ctx).Model(&securityRow{}).Where(map[string]any{"account_id": account}).Count(&count).Error
	return count > 0, err
}
func (s *Store) UpdateCharacter(ctx context.Context, account, id uint32, fn func(*game.Character) error) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		ref := CharacterRef{Account: account, ID: id}
		c, err := loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		oldID, oldSlot, oldName := c.ID, c.Slot, c.Name
		if err := fn(&c); err != nil {
			return err
		}
		if c.ID != oldID || c.Slot != oldSlot || c.Name != oldName {
			return errors.New("identity mutation is not allowed")
		}
		if err := c.Validate(); err != nil {
			return err
		}
		return saveCharacter(tx, ref, c)
	})
}
func (s *Store) DeleteCharacter(ctx context.Context, a Account, slot byte, code string) error {
	if slot < 1 || slot > 2 {
		return errors.New("invalid character slot")
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var security securityRow
		err := persistenceError(tx.Where(map[string]any{"account_id": a.ID}).Take(&security).Error)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && !verifyPassword(code, security.DeletionHash) {
			return ErrCredentials
		}
		if err := tx.Where(map[string]any{"id": a.CharacterID(slot), "account_id": a.ID, "slot": slot}).Delete(&characterRow{}).Error; err != nil {
			return err
		}
		if err := repairGuildLeaders(tx); err != nil {
			return err
		}
		var remaining int64
		if err := tx.Model(&characterRow{}).Where(map[string]any{"account_id": a.ID}).Count(&remaining).Error; err != nil {
			return err
		}
		if remaining == 0 {
			return tx.Where(map[string]any{"account_id": a.ID}).Delete(&securityRow{}).Error
		}
		return nil
	})
}
