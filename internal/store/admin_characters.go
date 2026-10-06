package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"time"
	"wonderland-gonline/internal/game"
)

const AdminPageLimit = 500

var ErrAdminConflict = errors.New("record changed; refresh before saving")

type AdminCharacter struct {
	AccountID uint32         `json:"account_id"`
	State     game.Character `json:"state"`
	Version   string         `json:"version"`
}

func CharacterVersion(c game.Character) string {
	raw, _ := json.Marshal(c.Clone())
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func adminCharacter(tx *gorm.DB, row characterRow) (AdminCharacter, error) {
	c, err := readCharacterState(tx, row)
	return AdminCharacter{AccountID: row.AccountID, State: c, Version: CharacterVersion(c)}, err
}
func (s *Store) AdminCharacters(ctx context.Context, query string, after uint32) ([]AdminCharacter, error) {
	var out []AdminCharacter
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		db := tx.Where(clause.Gt{Column: "id", Value: after})
		if query != "" {
			if id, err := strconv.ParseUint(query, 10, 32); err == nil {
				db = db.Where(clause.Eq{Column: "id", Value: uint32(id)})
			} else {
				db = db.Where(clause.Like{Column: "name", Value: "%" + query + "%"})
			}
		}
		var rows []characterRow
		if err := db.Omit("state").Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}}).Limit(AdminPageLimit).Find(&rows).Error; err != nil {
			return err
		}
		out = make([]AdminCharacter, 0, len(rows))
		for _, row := range rows {
			c, err := adminCharacter(tx, row)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return nil
	})
	return out, err
}
func (s *Store) AdminCharacter(ctx context.Context, id uint32) (AdminCharacter, error) {
	var out AdminCharacter
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var row characterRow
		if err := tx.Omit("state").Where(map[string]any{"id": id}).Take(&row).Error; err != nil {
			return err
		}
		var err error
		out, err = adminCharacter(tx, row)
		return err
	})
	return out, err
}

func adminAudit(tx *gorm.DB, action string, id any) error {
	return tx.Create(&auditRow{At: time.Now().UTC().Format(time.RFC3339), Action: action, Subject: fmt.Sprint(id)}).Error
}
func (s *Store) ReplaceAdminCharacter(ctx context.Context, id uint32, version string, next game.Character) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var row characterRow
		if err := tx.Omit("state").Where(map[string]any{"id": id}).Take(&row).Error; err != nil {
			return err
		}
		current, err := adminCharacter(tx, row)
		if err != nil {
			return err
		}
		if version == "" || version != current.Version {
			return ErrAdminConflict
		}
		if next.ID != id || next.Name != current.State.Name || next.Slot != current.State.Slot {
			return errors.New("character identity is immutable")
		}
		if err = next.Validate(); err != nil {
			return err
		}
		if err = saveCharacter(tx, CharacterRef{Account: row.AccountID, ID: id}, next); err != nil {
			return err
		}
		return adminAudit(tx, "administrator character edit", id)
	})
}
func (s *Store) DeleteAdminCharacter(ctx context.Context, id uint32, version string) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var row characterRow
		if err := tx.Omit("state").Where(map[string]any{"id": id}).Take(&row).Error; err != nil {
			return err
		}
		current, err := adminCharacter(tx, row)
		if err != nil {
			return err
		}
		if version == "" || version != current.Version {
			return ErrAdminConflict
		}
		if err = tx.Delete(&row).Error; err != nil {
			return err
		}

		if err = repairGuildLeaders(tx); err != nil {
			return err
		}
		var remaining int64
		if err = tx.Model(&characterRow{}).Where(map[string]any{"account_id": row.AccountID}).Count(&remaining).Error; err != nil {
			return err
		}
		if remaining == 0 {
			if err = tx.Where(map[string]any{"account_id": row.AccountID}).Delete(&securityRow{}).Error; err != nil {
				return err
			}
		}
		return adminAudit(tx, "administrator character deletion", id)
	})
}

type AuditEntry struct {
	ID      uint64 `json:"id"`
	At      string `json:"at"`
	Action  string `json:"action"`
	Subject string `json:"subject"`
}

func (s *Store) Audit(ctx context.Context) ([]AuditEntry, error) {
	var rows []auditRow
	err := s.orm.WithContext(ctx).Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: true}).Limit(AdminPageLimit).Find(&rows).Error
	out := make([]AuditEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, AuditEntry{r.ID, r.At, r.Action, r.Subject})
	}
	return out, err
}

type AdminFriendship struct {
	Character1 uint32 `json:"character1"`
	Character2 uint32 `json:"character2"`
}

func (s *Store) AdminFriends(ctx context.Context) ([]AdminFriendship, error) {
	var rows []friendshipRow
	err := s.orm.WithContext(ctx).Order("character1, character2").Limit(AdminPageLimit).Find(&rows).Error
	out := make([]AdminFriendship, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminFriendship{r.Character1, r.Character2})
	}
	return out, err
}
