package server

import (
	"context"
	"wonderland-gonline/internal/protocol"
)

// AC40 retains its legacy two-slot signature and echo, using the same new
// rank/base engine as AC23. Catastrophic failure delivers its junk output.
func (s *Server) alchemyCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.AlchemyRequestBytes {
		return protocol.ErrMalformed
	}
	if !gmIdle(c) || c.stall != nil {
		return nil
	}
	execution, err := s.executeCompounding(ctx, c, p[2:], true)
	if err != nil {
		if compoundingRejection(err) {
			return c.send(protocol.Builder{protocol.CommandAlchemy, p[1], protocol.AlchemyFailed}.U16(0))
		}
		return err
	}
	packets := [][]byte{
		{protocol.CommandInventory, protocol.InventoryRemove, p[2], 1},
		{protocol.CommandInventory, protocol.InventoryRemove, p[3], 1},
		c.character.Bag.AdditionPacket(execution.Additions),
		protocol.Builder{protocol.CommandAlchemy, p[1], protocol.AlchemySucceeded}.U16(execution.Outcome.ItemID)}
	return s.sendAll(c, append(packets, execution.SkillPackets...))
}
