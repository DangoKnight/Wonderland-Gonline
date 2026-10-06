package server

import (
	"context"
	"fmt"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// repairCommand ports AC36 and EquipmentRepairManager. The C# manager charges
// payment without clearing damage; Go fixes that and saves before any receipts.
// AC36 addresses the bag, despite the reference handler's worn-equipment summary.
// Caller holds worldMu; the world dispatcher guards loading, battle and trade.
func (s *Server) repairCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.RepairRequestBytes {
		return protocol.ErrMalformed
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	slot := p[2]
	reply := []byte{protocol.CommandEquipmentRepair, p[1], slot, protocol.RepairFailed}
	if slot < 1 || slot > game.BagSize {
		return c.send(reply)
	}
	item := c.character.Bag[slot-1]
	if item.Empty() {
		return s.sendAll(c, [][]byte{systemLine("Please select an item in your inventory to repair."), reply})
	}
	if _, known := s.Assets.Items[item.ID]; !known {
		return c.send(reply)
	}
	if item.Damage == 0 {
		return c.send(reply)
	}
	if c.character.ActiveVehicle != 0 && c.character.VehicleSlot == slot {
		return nil
	}
	next := c.character.Clone()
	tool, ok := next.RepairBagItem(slot)
	if !ok {
		return s.sendAll(c, [][]byte{systemLine("You need a Repair Wrench or 500 gold to repair your equipment!"), reply})
	}
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	var packets [][]byte
	if tool != 0 {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, tool, game.RepairToolsPerItem})
	} else {
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
	}
	// AC23:5 is additive. Remove the client's remaining old stack before adding
	// its repaired record, preserving forge metadata and avoiding duplication.
	count := next.Bag[slot-1].Count
	packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, count}, next.Bag.AdditionPacket([]game.Addition{{Slot: slot, Count: count}}))
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	effect := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateRepairEffect}.U32(next.ID).U16(game.RepairHammerEffectID)
	s.broadcastWorld(c, effect)
	reply[3] = protocol.RepairSucceeded
	return s.sendAll(c, [][]byte{effect, systemLine(fmt.Sprintf("[Blacksmith Repair] Successfully restored item #%d to maximum durability!", item.ID)), reply})
}
