package store

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"strings"
	"time"
	"wonderland-gonline/internal/game"
)

// LegacyAccount is an offline import record. Passwords are never report data.
// Plain credentials are converted to PBKDF2 before entering the destination.
type LegacyAccount struct {
	Account      Account
	Password     string `json:"-"`
	Characters   []game.Character
	DeletionCode string `json:"-"`
}

type LegacyFriendship struct{ Character1, Character2 uint32 }

// ImportLegacy requires an empty destination. Accounts, security and every typed
// character child commit together; it cannot overwrite a running installation.
func (s *Store) ImportLegacy(ctx context.Context, records []LegacyAccount, friendships ...LegacyFriendship) error {
	rows := make([]accountRow, len(records))
	security := make(map[uint32]string)
	for i, record := range records {
		a := record.Account
		a.Username = strings.ToLower(strings.TrimSpace(a.Username))
		if a.ID == 0 || a.ID >= SecondCharacterIDOffset-UserIDOffset || a.IM < 0 || a.IM > game.MaxMallPoints || a.IMBonus < 0 || a.IMBonus > game.MaxMallPoints || len(a.Email) > maxEmailBytes {
			return fmt.Errorf("legacy account %d: invalid identity or balances", a.ID)
		}
		if err := ValidateCredentials(a.Username, record.Password); err != nil {
			return fmt.Errorf("legacy account %d: incompatible native credentials", a.ID)
		}
		hash, err := hashPassword(record.Password)
		if err != nil {
			return err
		}
		rows[i] = accountRow{Account: a, PasswordHash: hash, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
		if record.DeletionCode != "" {
			if len(record.DeletionCode) < DeletionCodeMinBytes || len(record.DeletionCode) > DeletionCodeMaxBytes {
				return fmt.Errorf("legacy account %d: incompatible deletion code", a.ID)
			}
			security[a.ID], err = hashPassword(record.DeletionCode)
			if err != nil {
				return err
			}
		}
		for _, c := range record.Characters {
			if c.ID != a.CharacterID(c.Slot) {
				return fmt.Errorf("legacy character %d: account identity mismatch", c.ID)
			}
			if err = c.Validate(); err != nil {
				return fmt.Errorf("legacy character %d: %w", c.ID, err)
			}
		}
	}
	characterIDs := map[uint32]bool{}
	for _, record := range records {
		for _, c := range record.Characters {
			characterIDs[c.ID] = true
		}
	}
	friends := make([]friendshipRow, 0, len(friendships))
	seen := map[[2]uint32]bool{}
	counts := map[uint32]int{}
	for _, f := range friendships {
		a, b := friendPair(f.Character1, f.Character2)
		key := [2]uint32{a, b}
		if a == b || !characterIDs[a] || !characterIDs[b] || seen[key] {
			return fmt.Errorf("invalid legacy friendship")
		}
		counts[a]++
		counts[b]++
		if counts[a] > FriendLimit || counts[b] > FriendLimit {
			return ErrFriendLimit
		}
		seen[key] = true
		friends = append(friends, friendshipRow{Character1: a, Character2: b})
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&accountRow{}).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("legacy import requires an empty gameplay database")
		}
		for i, row := range rows {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			for _, c := range records[i].Characters {
				if err := tx.Create(&characterRow{ID: c.ID, AccountID: row.ID, Slot: c.Slot, Name: c.Name, State: []byte{}}).Error; err != nil {
					return err
				}
				if err := writeCharacterState(tx, c); err != nil {
					return err
				}
			}
			if hash := security[row.ID]; hash != "" {
				if err := tx.Create(&securityRow{AccountID: row.ID, DeletionHash: hash}).Error; err != nil {
					return err
				}
			}
		}
		if len(friends) > 0 {
			if err := tx.Create(&friends).Error; err != nil {
				return err
			}
		}
		return tx.Create(&auditRow{At: time.Now().UTC().Format(time.RFC3339), Action: "legacy_import", Subject: fmt.Sprintf("accounts=%d", len(records))}).Error
	})
}
