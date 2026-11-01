package server

import (
	"context"
	"errors"
	"slices"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func teleportPointPacket(point byte, l game.Location) []byte {
	return protocol.Builder{protocol.CommandTeleportScroll, protocol.TeleportScrollRecord, point}.U16(l.Map).U16(l.X).U16(l.Y)
}

func (s *Server) teleportScrollCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != 4 {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.TeleportScrollRecord && p[1] != protocol.TeleportScrollTravel {
		return ErrUnsupported
	}
	slot, point := p[2], p[3]
	if slot < 1 || slot > game.BagSize || point < 1 || point > store.MaxTeleportPoints {
		return protocol.ErrMalformed
	}
	if !gmIdle(c) || c.tentOwner != 0 {
		return nil
	}
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	if p[1] == protocol.TeleportScrollRecord {
		location := game.Location{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
		if err := s.Store.RecordTeleportPoint(ctx, ref, point, slot, location, s.Assets.Travel.Scrolls, *c.character); err != nil {
			if errors.Is(err, game.ErrInvalidItem) {
				return c.send(headBanner("A suitable unlocked Teleport Scroll is required."))
			}
			return err
		}
		return c.send(teleportPointPacket(point, location))
	}
	points, err := s.Store.TeleportPoints(ctx, ref)
	if err != nil {
		return err
	}
	dst, exists := points[point]
	if !exists {
		return c.send(headBanner("No location recorded for this point."))
	}
	return s.travelWithItem(ctx, c, slot, false, world.Destination{Map: dst.Map, X: dst.X, Y: dst.Y}, func(item game.Item) bool { return s.Assets.Travel.Scrolls[item.ID] >= point })
}

func (s *Server) bambooDragonfly(ctx context.Context, c *Session, destination, slot byte) error {
	dst, ok := s.Assets.Travel.Destinations[destination]
	if !ok || !gmIdle(c) || c.tentOwner != 0 || c.character.Map == dst.Map {
		return nil
	}
	return s.travelWithItem(ctx, c, slot, true, world.Destination{Map: dst.Map, X: dst.X, Y: dst.Y}, func(item game.Item) bool { return slices.Contains(s.Assets.Travel.BambooItems, item.ID) })
}

// travelWithItem makes the debit and new recovery destination durable together.
// No warp or success receipt is published before commit. Old-map departure uses
// the previous location even though inventory and the checkpoint already changed.
func (s *Server) travelWithItem(ctx context.Context, c *Session, slot byte, search bool, dst world.Destination, valid func(game.Item) bool) error {
	if !gmIdle(c) || c.tentOwner != 0 || c.character.Map == game.TentMapID || dst.Map == game.TentMapID {
		return nil
	}
	if _, exists := s.World.Map(dst.Map); !exists {
		return c.send(headBanner("Travel destination is unavailable."))
	}
	if c.openTent != nil {
		if err := s.closePlayerTent(ctx, c); err != nil {
			return err
		}
	}
	old := game.Location{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
	invalid := errors.New("travel item unavailable")
	consumeSlot := slot
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		usable := func(slot byte) bool {
			if slot < 1 || slot > game.BagSize {
				return false
			}
			item := next.Bag[slot-1]
			_, known := s.Assets.Items[item.ID]
			return known && !item.Empty() && !item.Locked && valid(item)
		}
		if !usable(consumeSlot) && search {
			for i := range next.Bag {
				if usable(byte(i + 1)) {
					consumeSlot = byte(i + 1)
					break
				}
			}
		}
		if next.Map != old.Map || !usable(consumeSlot) {
			return invalid
		}
		if err := next.Bag.Remove(consumeSlot, 1); err != nil {
			return err
		}
		next.Map, next.X, next.Y = dst.Map, dst.X, dst.Y
		next.TentReturn = nil
		return nil
	}, *c.character)
	if errors.Is(err, invalid) {
		return c.send(headBanner("A suitable unlocked travel item is required."))
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	if err := s.teleportPrelude(c); err != nil {
		return err
	}
	if err := c.send([]byte{protocol.CommandInventory, protocol.InventoryRemove, consumeSlot, 1}); err != nil {
		return err
	}
	c.character.Map, c.character.X, c.character.Y = old.Map, old.X, old.Y
	return s.teleportAfterSave(c, dst, 0)
}
