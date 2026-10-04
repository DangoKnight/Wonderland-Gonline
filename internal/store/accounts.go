package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var dummyPasswordHash = fmt.Sprintf("%s$%d$%s$%s", passwordHashAlgorithm, passwordIterations, strings.Repeat("0", passwordSaltBytes*2), strings.Repeat("0", passwordKeyBytes*2))

func (s *Store) Register(ctx context.Context, name, password, email string) (Account, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if err := ValidateCredentials(name, password); err != nil {
		return Account{}, err
	}
	if len(email) > maxEmailBytes {
		return Account{}, errors.New("email too long")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return Account{}, err
	}
	row := accountRow{Account: Account{Username: name, Email: email}, PasswordHash: hash, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := s.orm.WithContext(ctx).Create(&row).Error; err != nil {
		var count int64
		if s.orm.WithContext(ctx).Model(&accountRow{}).Where(map[string]any{"username": name}).Count(&count).Error == nil && count > 0 {
			return Account{}, ErrConflict
		}
		return Account{}, err
	}
	return row.Account, nil
}
func (s *Store) Authenticate(ctx context.Context, name, password string) (Account, error) {
	if ValidateCredentials(name, password) != nil {
		return Account{}, ErrCredentials
	}
	var row accountRow
	err := persistenceError(s.orm.WithContext(ctx).Where(map[string]any{"username": strings.ToLower(name)}).Take(&row).Error)
	if errors.Is(err, sql.ErrNoRows) {
		verifyPassword(password, dummyPasswordHash)
		return Account{}, ErrCredentials
	}
	if err != nil {
		return Account{}, err
	}
	if !verifyPassword(password, row.PasswordHash) || row.Banned {
		return Account{}, ErrCredentials
	}
	return row.Account, nil
}
func (s *Store) Accounts(ctx context.Context) ([]Account, error) {
	rows := []accountRow{}
	if err := s.orm.WithContext(ctx).Select("id", "username", "email", "banned", "im", "im_bonus", "gm_level").Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).Limit(adminAccountListLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Account, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.Account)
	}
	return result, nil
}
func (s *Store) SetBanned(ctx context.Context, id uint32, banned bool) error {
	return s.updateAccount(ctx, id, "banned", banned, fmt.Sprintf("ban=%t", banned))
}
func (s *Store) SetGMLevel(ctx context.Context, id uint32, level byte) error {
	return s.updateAccount(ctx, id, "gm_level", level, fmt.Sprintf("gm_level=%d", level))
}
func (s *Store) updateAccount(ctx context.Context, id uint32, column string, value any, action string) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		result := tx.Model(&accountRow{}).Where(map[string]any{"id": id}).Update(column, value)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return sql.ErrNoRows
		}
		return tx.Create(&auditRow{At: time.Now().UTC().Format(time.RFC3339), Action: action, Subject: fmt.Sprint(id)}).Error
	})
}

// GMLevels reads every account's privileges, without the administration list's
// display limit. Reload must also revoke sessions absent from this snapshot.
func (s *Store) GMLevels(ctx context.Context) (map[uint32]byte, error) {
	var rows []accountRow
	if err := s.orm.WithContext(ctx).Select("id", "gm_level").Find(&rows).Error; err != nil {
		return nil, err
	}
	levels := make(map[uint32]byte, len(rows))
	for _, row := range rows {
		levels[row.ID] = row.GMLevel
	}
	return levels, nil
}
