package server

import "wonderland-gonline/internal/protocol"

import (
	"context"
	"wonderland-gonline/internal/game"
)

// redeemVoucher is PetVoucherManager.TryRedeem. A recognized voucher is handled
// even on refusal. Bag consumption and the new pet are committed together.
func (s *Server) redeemVoucher(ctx context.Context, c *Session, slot, count byte, target uint16) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	id, known := s.Assets.PetVouchers[item.ID]
	if !known {
		return false, nil
	}
	refuse := func(text string) (bool, error) { return true, c.send(headBanner(text)) }
	if count != 1 || target != 0 {
		return refuse("Use one pet voucher at a time on your character.")
	}
	if item.Empty() || c.battle != nil {
		return refuse("Cannot use this pet voucher right now. Voucher retained.")
	}
	t, available := s.World.Template(uint32(id))
	if !available || !s.hasItem(item.ID) {
		return refuse("This pet has no available data. Voucher retained.")
	}
	if _, owned := c.character.Pet(uint32(id)); owned {
		return refuse("This pet is already in your party. Voucher retained.")
	}
	next := c.character.Clone()
	partySlot := next.FreePetSlot()
	if partySlot == 0 {
		return refuse("Your pet party is full. Free a slot first. Voucher retained.")
	}
	// Reserve the client slot on a copy, so a failed database commit cannot
	// consume a per-login roster slot. Hotel copies are allowed by the source.
	roster := &petRoster{slots: make(map[uint32]byte), synced: c.pets.synced}
	for id, slot := range c.pets.slots {
		roster.slots[id] = slot
	}
	if !roster.register(uint32(id)) {
		return refuse("No free pet slot is available. Voucher retained.")
	}
	pet := game.NewPet(uint32(id), s.npcName(uint32(id)), partySlot, t, s.Assets.Items)
	pet.EnsureSkills(t, s.hasSkill)
	next.Pets = append(next.Pets, pet)
	if err := next.Bag.Remove(slot, 1); err != nil {
		return true, err
	}
	if err := s.commit(ctx, c, next); err != nil {
		return true, err
	}
	c.pets = roster
	cs := roster.slot(pet.ID)
	packets := [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, pet.RecruitPacket(next.ID, t)}
	packets = append(packets, pet.ProgressionPackets(cs, s.Assets.Items)...)
	packets = append(packets, petNamePacket(next.ID, cs, pet.Name), []byte{protocol.CommandInventory, protocol.InventoryItemUse}, headBanner(pet.Name+" joined your party."))
	return true, s.sendAll(c, packets)
}
