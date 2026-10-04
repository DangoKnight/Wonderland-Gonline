package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
	"wonderland-go/internal/game"
)

const ParcelInboxLimit = 255
const ParcelSubjectMaxBytes = 100
const ParcelBodyMaxBytes = 255

var ErrParcelPending = errors.New("claim the attachments before deleting this letter")

type Parcel struct {
	ID         uint32 `gorm:"primaryKey"`
	SenderID   *uint32
	SenderName string
	ReceiverID uint32
	Subject    string
	Body       string
	Gold       uint32
	ItemID     uint16
	Count      byte
	Damage     byte
	Metadata   []byte
	SentAt     int64
	IsRead     bool
	Claimed    bool
}

func (Parcel) TableName() string { return "parcels" }
func (s *Store) Parcels(ctx context.Context, receiver uint32) ([]Parcel, error) {
	rows := []Parcel{}
	err := s.orm.WithContext(ctx).Where("receiver_id = ?", receiver).Order("id").Limit(ParcelInboxLimit).Find(&rows).Error
	return rows, err
}
func (s *Store) SendParcel(ctx context.Context, ref CharacterRef, receiver uint32, subject, body string, gold uint32, slot, count byte, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, error) {
	var next game.Character
	if receiver == ref.ID || !validSocialText(subject, ParcelSubjectMaxBytes) || !validSocialText(body, ParcelBodyMaxBytes) || (slot == 0) != (count == 0) {
		return next, errors.New("invalid parcel")
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var recipient characterRow
		if err := tx.Select("id").First(&recipient, receiver).Error; err != nil {
			return err
		}
		var inbox int64
		if err := tx.Model(&Parcel{}).Where("receiver_id = ?", receiver).Count(&inbox).Error; err != nil {
			return err
		}
		if inbox >= ParcelInboxLimit {
			return errors.New("recipient inbox is full")
		}
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if gold > next.Gold {
			return game.ErrTradeChanged
		}
		row := Parcel{SenderID: &ref.ID, SenderName: next.Name, ReceiverID: receiver, Subject: subject, Body: body, Gold: gold, Metadata: make([]byte, game.ItemMetadataBytes), SentAt: time.Now().UTC().UnixMilli()}
		if slot != 0 {
			if int(slot) > game.BagSize {
				return game.ErrInvalidItem
			}
			item := next.Bag[slot-1]
			if _, known := items[item.ID]; !known || item.Empty() || count > item.Count {
				return game.ErrInvalidItem
			}
			if next.ActiveVehicle == item.ID {
				return errors.New("dismount before mailing this vehicle")
			}
			row.ItemID, row.Count, row.Damage = item.ID, count, item.Damage
			copy(row.Metadata, item.Metadata[:])
			if err = next.Bag.Remove(slot, count); err != nil {
				return err
			}
		}
		next.Gold -= gold
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	return next, err
}
func (s *Store) ReadParcel(ctx context.Context, receiver, id uint32) (Parcel, error) {
	var row Parcel
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND receiver_id = ?", id, receiver).Take(&row).Error; err != nil {
			return err
		}
		row.IsRead = true
		return tx.Model(&row).Update("is_read", true).Error
	})
	return row, err
}
func (s *Store) ClaimParcel(ctx context.Context, ref CharacterRef, id uint32, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	var next game.Character
	var adds []game.Addition
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var row Parcel
		if err := tx.Where("id = ? AND receiver_id = ?", id, ref.ID).Take(&row).Error; err != nil {
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
		if row.Claimed {
			return nil
		}
		if uint64(next.Gold)+uint64(row.Gold) > game.MaxGold {
			return game.ErrTradeGoldLimit
		}
		if row.ItemID != 0 {
			item, known := items[row.ItemID]
			if !known || len(row.Metadata) != game.ItemMetadataBytes {
				return game.ErrInvalidItem
			}
			grant := game.Item{ID: row.ItemID, Count: row.Count, Damage: row.Damage}
			copy(grant.Metadata[:], row.Metadata)
			adds, err = next.Bag.Grant(grant, int(row.Count), item.StackLimit(), items)
			if err != nil {
				return err
			}
		}
		next.Gold += row.Gold
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		return tx.Model(&row).Update("claimed", true).Error
	})
	return next, adds, err
}
func (s *Store) DeleteParcel(ctx context.Context, receiver, id uint32) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var row Parcel
		if err := tx.Where("id = ? AND receiver_id = ?", id, receiver).Take(&row).Error; err != nil {
			return err
		}
		if !row.Claimed && (row.Gold != 0 || row.ItemID != 0) {
			return ErrParcelPending
		}
		return tx.Delete(&row).Error
	})
}
