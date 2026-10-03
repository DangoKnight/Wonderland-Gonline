package server

import (
	"context"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

const (
	alchemyUnitsPerIngredient = 1
	alchemyOutputQuantity     = 1
)

// alchemyCommand ports AC40. Recipes come from the SQL-loaded catalog. Unlike
// AC23, a missing recipe fails without consuming either ingredient. The legacy
// handler ignores recipe rates and echoes the subcommand for every operation.
// Caller holds worldMu; the dispatcher guards map loading, battles and trades.
func (s *Server) alchemyCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.AlchemyRequestBytes {
		return protocol.ErrMalformed
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	reply := protocol.Builder{protocol.CommandAlchemy, p[1], protocol.AlchemyFailed}.U16(0)
	first, second := p[2], p[3]
	if first < 1 || first > game.BagSize || second < 1 || second > game.BagSize || first == second {
		return c.send(reply)
	}
	if c.character.ActiveVehicle != 0 && (first == c.character.VehicleSlot || second == c.character.VehicleSlot) {
		return nil
	}
	a, b := c.character.Bag[first-1], c.character.Bag[second-1]
	if a.Empty() || b.Empty() {
		return c.send(reply)
	}
	if _, known := s.Assets.Items[a.ID]; !known {
		return c.send(reply)
	}
	if _, known := s.Assets.Items[b.ID]; !known {
		return c.send(reply)
	}
	result, ok := s.Assets.AlchemyResult(a.ID, b.ID)
	if !ok {
		return c.send(reply)
	}
	limit, known := s.stackLimit(result)
	if !known {
		return c.send(reply)
	}
	// Simulate both removals and the grant before committing. AddItem semantics
	// merge compatible fresh stacks; damaged or forged stacks retain metadata.
	next := c.character.Clone()
	if err := next.Bag.Remove(first, alchemyUnitsPerIngredient); err != nil {
		return c.send(reply)
	}
	if err := next.Bag.Remove(second, alchemyUnitsPerIngredient); err != nil {
		return c.send(reply)
	}
	adds, err := next.Bag.Grant(game.Item{ID: result}, alchemyOutputQuantity, limit)
	if err != nil {
		return c.send(reply)
	}
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	return s.sendAll(c, [][]byte{
		{protocol.CommandInventory, protocol.InventoryRemove, first, alchemyUnitsPerIngredient},
		{protocol.CommandInventory, protocol.InventoryRemove, second, alchemyUnitsPerIngredient},
		next.Bag.AdditionPacket(adds),
		protocol.Builder{protocol.CommandAlchemy, p[1], protocol.AlchemySucceeded}.U16(result),
	})
}
