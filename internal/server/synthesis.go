package server

import (
	"context"
	"fmt"
	"wonderland-gonline/internal/protocol"
)

// The chat shortcut shares the same SQL-owned rank/base engine as the UI.
func (s *Server) synthesize(ctx context.Context, c *Session, slots ...byte) error {
	if !gmIdle(c) || c.stall != nil {
		return nil
	}
	execution, err := s.executeCompounding(ctx, c, slots, true)
	if err != nil {
		if compoundingRejection(err) {
			return s.chatFeedback(c, err.Error())
		}
		return err
	}
	packets := [][]byte{}
	for _, slot := range slots {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, 1})
	}
	packets = append(packets, c.character.Bag.AdditionPacket(execution.Additions))
	packets = append(packets, execution.SkillPackets...)
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, fmt.Sprintf("Synthesis produced %s (rank %d).", s.Assets.Items[execution.Outcome.ItemID].Name, execution.Outcome.ResultRank))
}
