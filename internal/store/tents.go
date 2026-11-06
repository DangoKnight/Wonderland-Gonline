package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const TentItemLimit = 65535
const maxTentTreeNodes = (protocol.MaxPayload - protocol.TentTreeHeaderBytes) / protocol.TentTreeRecordBytes

func tentTreeNodes(t Tent) int {
	n := len(t.Items)
	for _, item := range t.Items {
		n += len(item.Tools)
	}
	return n
}

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
	Tools           []TentTool `gorm:"-"`
	OwnerID         uint32     `gorm:"primaryKey;autoIncrement:false"`
	Slot            uint16     `gorm:"primaryKey;autoIncrement:false"`
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
	if err != nil {
		return t, err
	}
	var tools []TentTool
	if err := tx.Where("owner_id = ?", owner).Order("host_slot,ordinal").Find(&tools).Error; err != nil {
		return t, err
	}
	byHost := map[uint16][]TentTool{}
	for _, tool := range tools {
		byHost[tool.HostSlot] = append(byHost[tool.HostSlot], tool)
	}
	for i := range t.Items {
		t.Items[i].Tools = byHost[t.Items[i].Slot]
	}
	return t, nil
}
func (s *Store) Tent(ctx context.Context, owner uint32) (Tent, error) {
	var t Tent
	err := s.transaction(ctx, func(tx *gorm.DB) error { var err error; t, err = loadTent(tx, owner); return err })
	return t, err
}

// ApplyTentSurface commits inventory consumption with the decor metadata. The
// owner/ref and exact instance are rechecked inside the same transaction.
func (s *Store) ApplyTentSurface(ctx context.Context, ref CharacterRef, slot byte, expected game.Item, wallpaper bool, floor byte, reservations ...game.Character) (game.Character, Tent, error) {
	var next game.Character
	var tent Tent
	if slot < 1 || slot > game.BagSize || expected.Empty() || expected.Locked || floor > 1 {
		return next, tent, game.ErrInvalidItem
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if next.Map != game.TentMapID || next.TentReturn == nil || next.Bag[slot-1] != expected {
			return game.ErrInvalidItem
		}
		tent, err = loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		if floor == 1 && tent.Type < tentExpandedType {
			return game.ErrInvalidItem
		}
		if err = next.Bag.Remove(slot, 1); err != nil {
			return err
		}
		if wallpaper {
			if floor == 0 {
				tent.Wallpaper = expected.ID
			} else {
				tent.Wallpaper2 = expected.ID
			}
		} else {
			if floor == 0 {
				tent.Floor = expected.ID
			} else {
				tent.Floor2 = expected.ID
			}
		}
		if err = tx.Omit("Items").Save(&tent).Error; err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	return next, tent, err
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
	return s.placeTentItem(ctx, ref, bagSlot, x, y, 0, expected, nil, false)
}

func (s *Store) PlaceNativeTentItem(ctx context.Context, ref CharacterRef, bagSlot byte, x, y uint16, floor byte, expected game.Item, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, error) {
	return s.placeTentItem(ctx, ref, bagSlot, x, y, floor, expected, items, true, reservations...)
}

func (s *Store) placeTentItem(ctx context.Context, ref CharacterRef, bagSlot byte, x, y uint16, floor byte, expected game.Item, items map[uint16]game.ItemDefinition, strict bool, reservations ...game.Character) (game.Character, error) {
	var next game.Character
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if expected.Locked {
			return game.ErrItemLocked
		}
		if bagSlot < 1 || bagSlot > game.BagSize || next.Bag[bagSlot-1] != expected {
			return game.ErrTradeChanged
		}
		tent, err := loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		if strict {
			if next.Map != game.TentMapID || next.TentReturn == nil {
				return game.ErrInvalidItem
			}
			if !tentPlacementAvailable(tent, TentItem{ItemID: expected.ID, X: x, Y: y, Floor: floor}, items, nil) {
				return ErrTentPlacement
			}
		}
		if len(tent.Items) >= TentItemLimit || tentTreeNodes(tent) >= maxTentTreeNodes {
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
		row := TentItem{OwnerID: ref.ID, Slot: index, ItemID: expected.ID, Count: 1, Damage: expected.Damage, Metadata: append([]byte(nil), expected.Metadata[:]...), X: x, Y: y, Floor: floor}
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
		adds, err = next.Bag.Grant(item, 1, def.StackLimit(), items)
		if err != nil {
			return err
		}
		for _, tool := range row.Tools {
			definition, known := items[tool.ItemID]
			if !known || len(tool.Metadata) != game.ItemMetadataBytes {
				return game.ErrInvalidItem
			}
			it := game.Item{ID: tool.ItemID, Damage: tool.Damage}
			copy(it.Metadata[:], tool.Metadata)
			part, err := next.Bag.Grant(it, 1, definition.StackLimit(), items)
			if err != nil {
				return err
			}
			adds = append(adds, part...)
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
