package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"wonderland-gonline/internal/game"
)

const (
	tentGridMinX            = 29
	tentGridMaxX            = 44
	tentGridMinY            = 38
	tentExpandedGridMinY    = 32
	tentGrandGridMinY       = 27
	tentGridMaxY            = 50
	tentExpandedType        = 2
	tentGrandType           = 3
	tentRotationMaximum     = 3
	tentPreferredFurnitureX = 42
	tentPreferredFurnitureY = 42
)

var ErrTentPlacement = errors.New("tent position is outside the floor or occupied")

func tentMinimumY(t Tent) int {
	minimumY := tentGridMinY
	if t.Enlarged || t.Type >= tentExpandedType {
		minimumY = tentExpandedGridMinY
	}
	if t.Type >= tentGrandType {
		minimumY = tentGrandGridMinY
	}
	return minimumY
}

func tentPlacementAvailable(t Tent, candidate TentItem, items map[uint16]game.ItemDefinition, ignore *uint16) bool {
	minimumY := tentMinimumY(t)
	if candidate.Floor > 1 || candidate.Floor == 1 && t.Type < tentExpandedType || candidate.Rotation > tentRotationMaximum || int(candidate.X) < tentGridMinX || int(candidate.X) > tentGridMaxX || int(candidate.Y) < minimumY || int(candidate.Y) > tentGridMaxY {
		return false
	}
	w, h := items[candidate.ItemID].Footprint()
	if candidate.Rotation == 1 || candidate.Rotation == 3 {
		w, h = h, w
	}
	x, y := int(candidate.X), int(candidate.Y)
	for _, other := range t.Items {
		if other.Floor != candidate.Floor || ignore != nil && other.Slot == *ignore {
			continue
		}
		ow, oh := items[other.ItemID].Footprint()
		if other.Rotation == 1 || other.Rotation == 3 {
			ow, oh = oh, ow
		}
		ox, oy := int(other.X), int(other.Y)
		if x < ox+ow && x+w > ox && y < oy+oh && y+h > oy {
			return false
		}
	}
	return true
}

// findTentFurnitureSpot prefers the desktop server's (42,42) anchor, then
// searches every allowed cell on the requested floor, including enlarged rows.
// A full floor rejects delivery so the transaction retains the recoverable job.
func findTentFurnitureSpot(t Tent, candidate TentItem, items map[uint16]game.ItemDefinition) (TentItem, bool) {
	candidate.X, candidate.Y = tentPreferredFurnitureX, tentPreferredFurnitureY
	if tentPlacementAvailable(t, candidate, items, nil) {
		return candidate, true
	}
	for y := tentMinimumY(t); y <= tentGridMaxY; y++ {
		for x := tentGridMinX; x <= tentGridMaxX; x++ {
			candidate.X, candidate.Y = uint16(x), uint16(y)
			if tentPlacementAvailable(t, candidate, items, nil) {
				return candidate, true
			}
		}
	}
	return TentItem{}, false
}

func nativeTentIndex(raw uint16) int {
	if raw == 0 {
		return 0
	}
	return int(raw) - 1
}

func (s *Store) MoveNativeTentItem(ctx context.Context, ref CharacterRef, rawIndex, x, y uint16, rotation byte, items map[uint16]game.ItemDefinition) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		c, err := loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if !game.IsTentMap(c.Map) || c.TentReturn == nil {
			return game.ErrInvalidItem
		}
		tent, err := loadTent(tx, ref.ID)
		if err != nil {
			return err
		}
		index := nativeTentIndex(rawIndex)
		if index >= len(tent.Items) {
			return game.ErrInvalidItem
		}
		row := tent.Items[index]
		row.X, row.Y, row.Rotation = x, y, rotation
		if !tentPlacementAvailable(tent, row, items, &row.Slot) {
			return ErrTentPlacement
		}
		return tx.Model(&row).Updates(map[string]any{"x": x, "y": y, "rotation": rotation}).Error
	})
}
