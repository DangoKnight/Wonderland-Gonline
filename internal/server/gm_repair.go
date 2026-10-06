package server

import (
	"context"
	"fmt"
	"strings"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// gmRepair ports GmManager.RepairAllItems. The caller holds worldMu and has
// checked GM authorization. Missing targets must not fall back to repairing self.
func (s *Server) gmRepair(ctx context.Context, actor *Session, arguments []string) error {
	arguments = strings.Fields(strings.Join(arguments, " "))
	if len(arguments) > 1 {
		return s.chatFeedback(actor, "Usage: /repair [character ID or name]")
	}
	target := actor
	if len(arguments) == 1 {
		target = s.findOnline(arguments[0])
		if target == nil {
			return s.chatFeedback(actor, "That character is offline or unavailable.")
		}
	}
	if !commandTravelAvailable(actor) || actor.trade != nil || !commandTravelAvailable(target) || target.trade != nil {
		return s.chatFeedback(actor, "Finish active interactions before repairing items.")
	}
	next := target.character.Clone()
	var additions []game.Addition
	var packets [][]byte
	repaired := 0
	for i, item := range next.Bag {
		if item.Empty() || item.Damage == 0 {
			continue
		}
		next.Bag[i].Damage = 0
		slot := byte(i + 1)
		// AC23:5 adds records; remove the old stack before adding its repair.
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, item.Count})
		additions = append(additions, game.Addition{Slot: slot, Count: item.Count})
		repaired++
	}
	gearChanged := false
	for i, item := range next.Equipment {
		if item.Empty() || item.Damage == 0 {
			continue
		}
		next.Equipment[i].Damage = 0
		gearChanged = true
		repaired++
	}
	if repaired == 0 {
		return s.chatFeedback(actor, fmt.Sprintf("%s has no damaged equipped or inventory items.", next.Name))
	}
	if err := s.commit(ctx, target, next); err != nil {
		return err
	}
	if len(additions) > 0 {
		packets = append(packets, next.Bag.AdditionPacket(additions))
	}
	if gearChanged {
		packets = append(packets, next.EquipmentPacket())
	}
	if err := s.sendAll(target, packets); err != nil {
		if target != actor {
			target.conn.Close()
			return s.chatFeedback(actor, "Repair saved; the target must reconnect to refresh items.")
		}
		return err
	}
	return s.chatFeedback(actor, fmt.Sprintf("Repaired %d equipped/inventory item stacks for %s.", repaired, next.Name))
}
