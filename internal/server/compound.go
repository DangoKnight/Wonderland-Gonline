package server

import (
	"context"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// compoundCommand is AC23.Recv14. Caller holds worldMu. Read the entire two-slot
// request before planning inventory changes; unsupported extra ingredients must
// never be silently consumed or ignored.
func (s *Server) compoundCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.CompoundRequestBytes {
		return protocol.ErrMalformed
	}
	if p[2] != protocol.CompoundIngredientSlots {
		return nil
	}
	first, second := p[3], p[4]
	if first < 1 || first > game.BagSize || second < 1 || second > game.BagSize || first == second {
		return nil
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	if c.character.ActiveVehicle != 0 && (first == c.character.VehicleSlot || second == c.character.VehicleSlot) {
		return nil
	}
	a, b := c.character.Bag[first-1], c.character.Bag[second-1]
	if a.Empty() || b.Empty() {
		return nil
	}
	if _, ok := s.Assets.Items[a.ID]; !ok {
		return nil
	}
	if _, ok := s.Assets.Items[b.ID]; !ok {
		return nil
	}
	result := s.Assets.CompoundResult(a.ID, b.ID)
	if _, ok := s.Assets.Items[result]; !ok {
		return nil
	}
	next := c.character.Clone()
	target, err := next.Bag.Compound(first, second, result, s.Assets.Items)
	if err != nil {
		return nil
	}
	if err = s.commit(ctx, c, next); err != nil {
		return err
	}
	packets := [][]byte{
		{protocol.CommandInventory, protocol.InventoryRemove, first, 1},
		{protocol.CommandInventory, protocol.InventoryRemove, second, 1},
		protocol.Builder{protocol.CommandInventory, protocol.InventoryCompoundResult, target}.U16(result).U8(1).Bytes(make([]byte, protocol.CompoundResultReservedBytes)),
		protocol.Builder{protocol.CommandInventory, protocol.InventoryCompoundSuccess}.U16(result).U8(1).U8(target),
	}
	if err = s.sendAll(c, packets); err != nil {
		return err
	}
	animation := protocol.Builder{protocol.CommandInventory, protocol.InventoryCompoundAnimation}.U32(next.ID)
	s.broadcastWorld(c, animation)
	return c.send(animation)
}
