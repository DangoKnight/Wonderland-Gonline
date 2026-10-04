package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
	"wonderland-go/internal/game"
)

const adminMailSubjectMaxBytes = 100

type AdminMail struct {
	ID         uint64 `json:"id" gorm:"primaryKey"`
	ReceiverID uint32 `json:"receiver_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	Gold       uint32 `json:"gold"`
	ItemID     uint16 `json:"item_id"`
	Count      byte   `json:"count"`
	Claimed    bool   `json:"claimed"`
	Delivered  bool   `json:"delivered"`
	At         int64  `json:"at"`
}

func (AdminMail) TableName() string { return "admin_mail" }
func (s *Store) AdminMailHistory(ctx context.Context) ([]AdminMail, error) {
	rows := []AdminMail{}
	err := s.orm.WithContext(ctx).Order("id DESC").Limit(AdminPageLimit).Find(&rows).Error
	return rows, err
}
func (s *Store) SendAdminMail(ctx context.Context, receivers []uint32, message AdminMail) error {
	if message.Subject == "" || len(message.Subject) > adminMailSubjectMaxBytes || ValidateMailContent([]byte(message.Subject+"\n"+message.Body)) != nil || (message.ItemID == 0) != (message.Count == 0) || len(receivers) == 0 || len(receivers) > AdminPageLimit {
		return errors.New("invalid GM mail")
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		seen := map[uint32]bool{}
		for _, id := range receivers {
			if seen[id] {
				return errors.New("duplicate recipient")
			}
			seen[id] = true
			var receiver characterRow
			if err := tx.Select("id").Where(map[string]any{"id": id}).Take(&receiver).Error; err != nil {
				return err
			}
			row := message
			row.ID = 0
			row.ReceiverID = id
			row.At = time.Now().UTC().UnixMilli()
			row.Claimed, row.Delivered = false, false
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return adminAudit(tx, "GM mail dispatch", len(receivers))
	})
}
func (s *Store) PendingAdminMail(ctx context.Context, id uint32) ([]AdminMail, error) {
	rows := []AdminMail{}
	err := s.orm.WithContext(ctx).Where(map[string]any{"receiver_id": id, "delivered": false}).Order("id").Limit(textMailBatchLimit).Find(&rows).Error
	return rows, err
}

const textMailBatchLimit = 32

func (s *Store) ClaimAdminMail(ctx context.Context, ref CharacterRef, id uint64, maxStack byte, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	var next game.Character
	var adds []game.Addition
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var message AdminMail
		if err := tx.Where(map[string]any{"id": id, "receiver_id": ref.ID}).Take(&message).Error; err != nil {
			return err
		}
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if message.Claimed {
			return nil
		}
		if uint64(next.Gold)+uint64(message.Gold) > game.MaxGold {
			return errors.New("gold limit reached")
		}
		next.Gold += message.Gold
		if message.ItemID != 0 {
			adds, err = next.Bag.Grant(game.Item{ID: message.ItemID}, int(message.Count), maxStack, items)
			if err != nil {
				return err
			}
		}
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		return tx.Model(&message).Update("claimed", true).Error
	})
	return next, adds, err
}
func (s *Store) MarkAdminMailDelivered(ctx context.Context, receiver uint32, id uint64) error {
	return s.orm.WithContext(ctx).Model(&AdminMail{}).Where(map[string]any{"id": id, "receiver_id": receiver, "claimed": true}).Update("delivered", true).Error
}
func (s *Store) DeleteAdminMail(ctx context.Context, id uint64) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Delete(&AdminMail{}, id).Error; err != nil {
			return err
		}
		return adminAudit(tx, "GM mail deletion", id)
	})
}
