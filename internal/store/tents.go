package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"wonderland-go/internal/game"
)

const TentItemLimit = 65535

type Tent struct {
	OwnerID    uint32 `gorm:"primaryKey;autoIncrement:false"`
	Locked     bool
	Enlarged   bool
	Type       byte
	Floor      uint16
	Floor2     uint16
	Wallpaper2 uint16
	Wallpaper  uint16
	Items      []TentItem `gorm:"-"`
}

func (Tent) TableName() string { return "tents" }

type TentItem struct {
	OwnerID         uint32 `gorm:"primaryKey;autoIncrement:false"`
	Slot            uint16 `gorm:"primaryKey;autoIncrement:false"`
	ItemID          uint16
	Count           byte
	Damage          byte
	Metadata        []byte
	X, Y            uint16
	Floor, Rotation byte
}

func (TentItem) TableName() string { return "tent_items" }
func loadTent(tx *gorm.DB, owner uint32) (Tent, error) {
	var t Tent
	if err := tx.First(&t, owner).Error; err != nil {
		return t, err
	}
	err := tx.Where("owner_id = ?", owner).Order("slot").Find(&t.Items).Error
	return t, err
}
func (s *Store) Tent(ctx context.Context, owner uint32) (Tent, error) {
	var t Tent
	err := s.transaction(ctx, func(tx *gorm.DB) error { var err error; t, err = loadTent(tx, owner); return err })
	return t, err
}
func (s *Store) EnsureTent(ctx context.Context, ref CharacterRef, floor, wallpaper uint16, defaults []TentItem) (Tent, error) {
	var result Tent
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := loadCharacter(tx, ref); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&Tent{}).Where("owner_id = ?", ref.ID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := tx.Create(&Tent{OwnerID: ref.ID, Floor: floor, Wallpaper: wallpaper}).Error; err != nil {
				return err
			}
			for i, row := range defaults {
				row.OwnerID = ref.ID
				row.Slot = uint16(i)
				row.Count = 1
				row.Metadata = make([]byte, game.ItemMetadataBytes)
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
		}
		var err error
		result, err = loadTent(tx, ref.ID)
		return err
	})
	return result, err
}
func (s *Store) PlaceTentItem(ctx context.Context, ref CharacterRef, bagSlot byte, x, y uint16, expected game.Item) (game.Character, error) {
	var next game.Character
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if bagSlot < 1 || bagSlot > game.BagSize || next.Bag[bagSlot-1] != expected {
			return game.ErrTradeChanged
		}
		tent, err := loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		if len(tent.Items) >= TentItemLimit {
			return errors.New("tent is full")
		}
		index := uint16(0)
		if len(tent.Items) > 0 {
			last := tent.Items[len(tent.Items)-1].Slot
			if last >= TentItemLimit {
				return errors.New("tent furniture index limit reached")
			}
			index = last + 1
		}
		if err = next.Bag.Remove(bagSlot, 1); err != nil {
			return err
		}
		row := TentItem{OwnerID: ref.ID, Slot: index, ItemID: expected.ID, Count: 1, Damage: expected.Damage, Metadata: append([]byte(nil), expected.Metadata[:]...), X: x, Y: y}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	return next, err
}
func (s *Store) MoveTentItem(ctx context.Context, ref CharacterRef, index uint16, x, y uint16, floor, rotation byte) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := loadCharacter(tx, ref); err != nil {
			return err
		}
		tent, err := loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		if int(index) >= len(tent.Items) {
			return game.ErrInvalidItem
		}
		row := tent.Items[index]
		return tx.Model(&row).Updates(map[string]any{"x": x, "y": y, "floor": floor, "rotation": rotation}).Error
	})
}
func (s *Store) PickUpTentItem(ctx context.Context, ref CharacterRef, index uint16, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	var next game.Character
	var adds []game.Addition
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		tent, err := loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		if int(index) >= len(tent.Items) {
			return game.ErrInvalidItem
		}
		row := tent.Items[index]
		def, known := items[row.ItemID]
		if !known || len(row.Metadata) != game.ItemMetadataBytes {
			return game.ErrInvalidItem
		}
		item := game.Item{ID: row.ItemID, Damage: row.Damage}
		copy(item.Metadata[:], row.Metadata)
		adds, err = next.Bag.Grant(item, 1, def.StackLimit())
		if err != nil {
			return err
		}
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
	return next, adds, err
}
func (s *Store) SetTentLocked(ctx context.Context, ref CharacterRef, locked bool) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := loadCharacter(tx, ref); err != nil {
			return err
		}
		t, err := loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		return tx.Model(&t).Update("locked", locked).Error
	})
}
