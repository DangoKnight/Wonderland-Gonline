package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"wonderland-go/internal/game"
)

// Persistence models map the existing schema; game state remains its JSON blob.
type accountRow struct {
	Account      `gorm:"embedded"`
	PasswordHash string
	CreatedAt    string `gorm:"autoCreateTime:false"`
}

func (accountRow) TableName() string { return "accounts" }

type characterRow struct {
	ID        uint32 `gorm:"primaryKey;autoIncrement:false"`
	AccountID uint32
	Slot      byte
	Name      string
	State     []byte
}

func (characterRow) TableName() string { return "characters" }

type securityRow struct {
	AccountID    uint32 `gorm:"primaryKey;autoIncrement:false"`
	DeletionHash string
}

func (securityRow) TableName() string { return "account_security" }

type settingRow struct {
	Key   string `gorm:"primaryKey"`
	Value string
}

func (settingRow) TableName() string { return "settings" }

type auditRow struct {
	ID                  uint64 `gorm:"primaryKey"`
	At, Action, Subject string
}

func (auditRow) TableName() string { return "audit" }

type friendshipRow struct {
	Character1 uint32 `gorm:"primaryKey;autoIncrement:false"`
	Character2 uint32 `gorm:"primaryKey;autoIncrement:false"`
}

func (friendshipRow) TableName() string { return "friendships" }

// Keep the Store's established errors so admin and gameplay callers stay stable.
func persistenceError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sql.ErrNoRows
	}
	return err
}
func (s *Store) transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return persistenceError(s.orm.WithContext(ctx).Transaction(fn))
}
func loadCharacter(tx *gorm.DB, ref CharacterRef) (game.Character, error) {
	var row characterRow
	if err := tx.Where(map[string]any{"id": ref.ID, "account_id": ref.Account}).Take(&row).Error; err != nil {
		return game.Character{}, persistenceError(err)
	}
	var c game.Character
	err := json.Unmarshal(row.State, &c)
	return c, err
}
func saveCharacter(tx *gorm.DB, ref CharacterRef, c game.Character) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	result := tx.Model(&characterRow{}).Where(map[string]any{"id": ref.ID, "account_id": ref.Account}).Update("state", raw)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func decodeCharacters(rows []characterRow) ([]game.Character, error) {
	result := make([]game.Character, 0, len(rows))
	for _, row := range rows {
		var c game.Character
		if err := json.Unmarshal(row.State, &c); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}
