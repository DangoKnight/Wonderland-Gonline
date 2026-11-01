package server

import (
	"context"
	"errors"
	"fmt"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// openBoxedItem atomically exchanges a deterministic box for its SQL-defined
// contents. Failed delivery retains the box, including its instance metadata.
func (s *Server) openBoxedItem(ctx context.Context, c *Session, slot byte) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	box, known := s.Assets.BoxedItems[item.ID]
	if !known {
		return false, nil
	}
	if item.Locked {
		return true, c.send(headBanner(game.ErrItemLocked.Error()))
	}
	if item.Empty() || c.event != nil || c.battle != nil || c.storm || c.beach != nil {
		return true, nil
	}
	limit, known := s.stackLimit(box.ItemID)
	if !known || box.Quantity == 0 {
		return true, c.send(headBanner("This box's contents are unavailable. Box retained."))
	}
	var adds []game.Addition
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		current := next.Bag[slot-1]
		if current.ID != item.ID || current.Empty() || current.Locked {
			return game.ErrInvalidItem
		}
		if err := next.Bag.Remove(slot, 1); err != nil {
			return err
		}
		var err error
		adds, err = next.Bag.Grant(game.Item{ID: box.ItemID}, int(box.Quantity), limit, s.Assets.Items)
		return err
	}, *c.character)
	if errors.Is(err, game.ErrInventoryFull) || errors.Is(err, game.ErrInvalidItem) {
		return true, c.send(headBanner("Cannot open this box. Check inventory space and ownership."))
	}
	if err != nil {
		return true, err
	}
	s.adoptSavedCharacter(c, next)
	return true, s.sendAll(c, [][]byte{
		{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, next.Bag.AdditionPacket(adds),
		{protocol.CommandInventory, protocol.InventoryItemUse},
		headBanner(fmt.Sprintf("Received [%s] x%d.", s.itemName(box.ItemID), box.Quantity)),
	})
}
