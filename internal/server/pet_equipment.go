package server

import "wonderland-go/internal/protocol"

import (
	"context"
	"wonderland-go/internal/game"
)

// changePetEquipment is AC23:17/18 and Inventory.TryEquipPet/TryUnequipPet.
// The shared equipment rules retain all item metadata during swaps.
func (s *Server) changePetEquipment(ctx context.Context, c *Session, slot, from, to byte, wear bool) error {
	if c.battle != nil {
		return nil
	}
	i := s.clientPet(c, slot)
	if i < 0 {
		return nil
	}
	next := c.character.Clone()
	pet := &next.Pets[i]
	before := pet.Combat(s.Assets.Items)
	transfer := game.Character{Bag: next.Bag, Equipment: pet.Equipment}
	var err error
	packet := []byte{protocol.CommandInventory, protocol.InventoryWireCode23, slot, from}
	if wear {
		if from < 1 || from > game.BagSize {
			return nil
		}
		equipSlot := s.Assets.Items[next.Bag[from-1].ID].EquipSlot
		if equipSlot >= 1 && equipSlot <= 6 {
			previous := pet.Equipment[equipSlot-1]
			if previous.ID != 0 && !s.hasItem(previous.ID) {
				return nil
			}
		}
		err = transfer.Wear(from, s.Assets.Items)
	} else {
		if from < 1 || from > 6 || !s.hasItem(pet.Equipment[from-1].ID) {
			return nil
		}
		err = transfer.Unwear(from, to, s.Assets.Items)
		packet = []byte{protocol.CommandInventory, protocol.InventoryWireCode22, slot, from, to}
	}
	if err != nil {
		return nil
	}
	next.Bag, pet.Equipment = transfer.Bag, transfer.Equipment
	pet.Normalize(s.Assets.Items, false)
	after := pet.Combat(s.Assets.Items)
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	packets := append([][]byte{packet}, pet.ProgressionPackets(slot, s.Assets.Items)...)
	if text := game.ChangeBanner(pet.Name+" equipment: ", before, after); text != "" {
		packets = append(packets, headBanner(text))
	}
	return s.sendAll(c, packets)
}
