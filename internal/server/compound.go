package server

import (
	"context"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// AC23:14/87/101 select Primary/Junior/Superior: count followed by ordered, distinct bag anchors. Two to five total
// inputs are accepted; books are excluded from the minimum material count.
func (s *Server) compoundCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < protocol.CompoundRequestHeaderBytes {
		return protocol.ErrMalformed
	}
	count := int(p[2])
	if count < protocol.CompoundIngredientSlots || count > protocol.CompoundMaximumIngredients || len(p) != protocol.CompoundRequestHeaderBytes+count {
		return protocol.ErrMalformed
	}
	if !gmIdle(c) || c.stall != nil {
		return nil
	}
	slots := p[protocol.CompoundRequestHeaderBytes:]
	tier := game.AlchemyPrimary
	switch p[1] {
	case protocol.InventoryCompoundJunior:
		tier = game.AlchemyJunior
	case protocol.InventoryCompoundSuperior:
		tier = game.AlchemySuperior
	}
	execution, err := s.executeCompounding(ctx, c, slots, false, tier)
	if err != nil {
		if compoundingRejection(err) {
			return c.send(systemLine(err.Error()))
		}
		return err
	}
	result := execution.Outcome.ItemID
	packets := make([][]byte, 0, len(slots)+2+len(execution.SkillPackets))
	for _, slot := range slots {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, 1})
	}
	packets = append(packets,
		protocol.Builder{protocol.CommandInventory, protocol.InventoryCompoundResult, execution.Target}.U16(result).U8(1).Bytes(make([]byte, protocol.CompoundResultReservedBytes)),
		protocol.Builder{protocol.CommandInventory, protocol.InventoryCompoundSuccess}.U16(result).U8(1).U8(execution.Target))
	packets = append(packets, execution.SkillPackets...)
	return s.sendAll(c, packets)
}
