package store

import (
	"context"
	"encoding/binary"
	"errors"

	"gorm.io/gorm"
	"wonderland-gonline/internal/game"
)

const TentItemLimit = 65535

// tentBenchItemID is the Work Platform that accepts accessory tools (AC 62:4).
const tentBenchItemID = 38049

// tentToolLimit is the number of accessory tools one appliance can hold.
const tentToolLimit = 10

// Placement rejections of the desktop server (AC 62:1 and AC 62:3), with the
// client messages it sends.
var (
	ErrTentBounds    = errors.New("Gecersiz konum!")
	ErrTentOccupied  = errors.New("Alan dolu!")
	ErrTentAppliance = errors.New("Bu esyaya arac takilamaz.")
	ErrTentToolLimit = errors.New("Bu esyaya daha fazla arac takilamaz.")
	ErrTentTool      = errors.New("Bu arac zaten takili.")
)

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
	// Tools are the accessory tool IDs attached to this item, u16 little endian each.
	Tools []byte `gorm:"default:''"`
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

// tentRects are the footprints of the tent's items, leaving out the item at
// skip (-1 keeps all of them).
func tentRects(items []TentItem, defs map[uint16]game.ItemDefinition, skip int) []game.TentRect {
	rects := make([]game.TentRect, 0, len(items))
	for i, row := range items {
		if i == skip {
			continue
		}
		w, h := game.TentFootprint(defs[row.ItemID], row.Rotation)
		rects = append(rects, game.TentRect{X: int(row.X), Y: int(row.Y), W: w, H: h, Floor: row.Floor})
	}
	return rects
}

// checkTentPlacement applies the desktop server's bounds and occupancy rules to
// an item of the given definition at a rotation.
func checkTentPlacement(tent Tent, items []TentItem, defs map[uint16]game.ItemDefinition, skip int, def game.ItemDefinition, x, y uint16, rotation byte) error {
	if !game.TentInFloorBounds(int(x), int(y), tent.Type, tent.Enlarged) {
		return ErrTentBounds
	}
	w, h := game.TentFootprint(def, rotation)
	candidate := game.TentRect{X: int(x), Y: int(y), W: w, H: h, Floor: 0}
	for _, r := range tentRects(items, defs, skip) {
		if game.TentOverlaps(r, candidate) {
			return ErrTentOccupied
		}
	}
	return nil
}

func (s *Store) PlaceTentItem(ctx context.Context, ref CharacterRef, bagSlot byte, x, y uint16, expected game.Item, defs map[uint16]game.ItemDefinition) (game.Character, error) {
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
		if err = checkTentPlacement(tent, tent.Items, defs, -1, defs[expected.ID], x, y, 0); err != nil {
			return err
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

// ApplyTentSurface uses a wall or floor item from the bag: the item is consumed
// and its ID becomes the tent's wallpaper or floor (floor 0 or 1).
func (s *Store) ApplyTentSurface(ctx context.Context, ref CharacterRef, bagSlot byte, expected game.Item, floor byte, wall bool) (game.Character, error) {
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
		if err = next.Bag.Remove(bagSlot, 1); err != nil {
			return err
		}
		if err = applyTentSurfaceTx(tx, ref.ID, expected.ID, floor, wall); err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	return next, err
}

// applyTentSurfaceTx sets a wall or floor value of a tent inside a transaction.
// The manufacturing path uses floor 0.
func applyTentSurfaceTx(tx *gorm.DB, owner uint32, itemID uint16, floor byte, wall bool) error {
	var column string
	switch {
	case wall && floor == 1:
		column = "wallpaper2"
	case wall:
		column = "wallpaper"
	case floor == 1:
		column = "floor2"
	default:
		column = "floor"
	}
	return tx.Model(&Tent{}).Where("owner_id = ?", owner).Update(column, itemID).Error
}

func (s *Store) MoveTentItem(ctx context.Context, ref CharacterRef, index uint16, x, y uint16, floor, rotation byte, defs map[uint16]game.ItemDefinition) error {
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
		if err = checkTentPlacement(tent, tent.Items, defs, int(index), defs[row.ItemID], x, y, rotation); err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{"x": x, "y": y, "floor": floor, "rotation": rotation}).Error
	})
}

// AttachTentTool puts a tool from the bag onto a workbench (AC 62:4).
func (s *Store) AttachTentTool(ctx context.Context, ref CharacterRef, bagSlot byte, expected game.Item, hostIndex uint16) (game.Character, error) {
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
		if int(hostIndex) >= len(tent.Items) {
			return game.ErrInvalidItem
		}
		host := tent.Items[hostIndex]
		if host.ItemID != tentBenchItemID {
			return ErrTentAppliance
		}
		tools := decodeTentTools(host.Tools)
		for _, t := range tools {
			if t == expected.ID {
				return ErrTentTool
			}
		}
		if len(tools) >= tentToolLimit {
			return ErrTentToolLimit
		}
		if err = next.Bag.Remove(bagSlot, 1); err != nil {
			return err
		}
		tools = append(tools, expected.ID)
		if err = tx.Model(&host).Update("tools", encodeTentTools(tools)).Error; err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	return next, err
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
		// Accessory tools attached to the appliance come back to the bag with it.
		for _, tool := range decodeTentTools(row.Tools) {
			toolDef, ok := items[tool]
			if !ok {
				return game.ErrInvalidItem
			}
			granted, grantErr := next.Bag.Grant(game.Item{ID: tool}, 1, toolDef.StackLimit(), items)
			if grantErr != nil {
				return grantErr
			}
			adds = append(adds, granted...)
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

func decodeTentTools(b []byte) []uint16 {
	out := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		out = append(out, binary.LittleEndian.Uint16(b[i:]))
	}
	return out
}

func encodeTentTools(tools []uint16) []byte {
	out := make([]byte, 2*len(tools))
	for i, t := range tools {
		binary.LittleEndian.PutUint16(out[2*i:], t)
	}
	return out
}
