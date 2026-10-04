package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ResetPassword changes the credential and its audit entry atomically. Neither
// plaintext nor the password hash is included in the audit record.
func (s *Store) ResetPassword(ctx context.Context, id uint32, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.updateAccount(ctx, id, "password_hash", hash, "password_reset")
}

// DeleteAccount removes all owned state and records the deletion in one transaction.
func (s *Store) DeleteAccount(ctx context.Context, id uint32) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var owner accountRow
		if err := tx.Select("id").Where(map[string]any{"id": id}).Take(&owner).Error; err != nil {
			return err
		}
		if err := tx.Where(map[string]any{"account_id": id}).Delete(&characterRow{}).Error; err != nil {
			return err
		}
		if err := repairGuildLeaders(tx); err != nil {
			return err
		}
		if err := tx.Where(map[string]any{"account_id": id}).Delete(&securityRow{}).Error; err != nil {
			return err
		}
		result := tx.Where(map[string]any{"id": id}).Delete(&accountRow{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return sql.ErrNoRows
		}
		return tx.Create(&auditRow{At: time.Now().UTC().Format(time.RFC3339), Action: "account_delete", Subject: fmt.Sprint(id)}).Error
	})
}
