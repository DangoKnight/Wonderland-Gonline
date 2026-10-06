package server

import (
	"context"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func vehicleMountPacket(c *game.Character) []byte {
	return protocol.Builder{protocol.CommandPetControl, protocol.PetControlVehicleMount, c.VehicleSlot}.U32(c.ID).U16(c.ActiveVehicle)
}
func vehicleDismountPacket(c *game.Character) []byte {
	return protocol.Builder{protocol.CommandPetControl, protocol.PetControlWireCode11, c.VehicleSlot}.U32(c.ID)
}

// sendVehiclePackets sends the native owner/map sequence, without changing state.
// Caller holds worldMu. Receipt failures cannot undo an already committed change.
func (s *Server) sendVehiclePackets(c *Session, packets [][]byte) error {
	for _, p := range packets {
		if c.ready {
			s.broadcastWorld(c, p)
		}
		if err := c.send(p); err != nil {
			return err
		}
	}
	return nil
}

// vehicleCommand ports AC15:7/9 boarding, 14 placement, 10 landing and 13 ACK.
// Placement only announces coordinates; confirmation owns the active state.
func (s *Server) vehicleCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if p[1] == protocol.PetControlVehicleNoOp {
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return nil
	}
	if len(p) != 5 {
		return protocol.ErrMalformed
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	slot, id := p[2], uint16(p[3])|uint16(p[4])<<8
	switch p[1] {
	case protocol.PetControlPlaceVehicle:
		if _, ok := c.character.Vehicle(slot, id, s.Assets.Items); !ok {
			return nil
		}
		return c.send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlVehiclePosition, slot}.U32(c.character.ID).U16(id).U32(uint32(c.character.X)).U32(uint32(c.character.Y)))
	case protocol.PetControlBoardVehicle, protocol.PetControlBoardVehicleAlternate:
		if _, ok := c.character.Vehicle(slot, id, s.Assets.Items); !ok {
			return nil
		}
		if c.character.ActiveVehicle == id && c.character.VehicleSlot == slot {
			return nil
		}
		next := c.character.Clone()
		var unride [][]byte
		if next.ActiveMount != 0 {
			unride = unmountPackets(next.ID)
		}
		next.ActiveMount = 0
		next.ActiveVehicle, next.VehicleSlot = id, slot
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		return s.sendVehiclePackets(c, append(unride, vehicleMountPacket(c.character)))
	case protocol.PetControlDismountVehicle:
		// The discriminator is not an inventory slot. The owned slot controls removal.
		if id == 0 || c.character.ActiveVehicle != id {
			return nil
		}
		if id == game.DisposableRaftItemID {
			return s.wreckVehicle(ctx, c)
		}
		return s.dismountVehicle(ctx, c)
	default:
		return ErrUnsupported
	}
}

func (s *Server) dismountVehicle(ctx context.Context, c *Session) error {
	if c.character.ActiveVehicle == 0 {
		return nil
	}
	next := c.character.Clone()
	next.ActiveVehicle, next.VehicleSlot = 0, 0
	// commit emits the old slot's dismount only after the save.
	return s.commit(ctx, c, next)
}

// wreckVehicle destroys one item in the mounted slot. Only rafts are disposable;
// a stale slot or another vehicle dismounts without consuming any replacement.
func (s *Server) wreckVehicle(ctx context.Context, c *Session) error {
	old := *c.character
	if old.ActiveVehicle == 0 {
		return nil
	}
	if old.VehicleSlot < 1 || old.VehicleSlot > game.BagSize {
		return s.dismountVehicle(ctx, c)
	}
	item := old.Bag[old.VehicleSlot-1]
	if item.Empty() || item.ID != old.ActiveVehicle || s.Assets.Items[item.ID].Type != game.VehicleType || !game.Raft(item.ID) {
		return s.dismountVehicle(ctx, c)
	}
	next := old.Clone()
	next.ActiveVehicle, next.VehicleSlot = 0, 0
	if err := next.Bag.Remove(old.VehicleSlot, 1); err != nil {
		return err
	}
	// Custom packet ordering: deletion, break, dismount, then the scene-load gate.
	if err := s.commitState(ctx, c, next); err != nil {
		return err
	}
	if err := c.send([]byte{protocol.CommandInventory, protocol.InventoryRemove, old.VehicleSlot, 1}); err != nil {
		return err
	}
	packets := [][]byte{protocol.Builder{protocol.CommandPetControl, protocol.PetControlRemoveVehicle}.U32(old.ID).U16(old.ActiveVehicle), vehicleDismountPacket(&old)}
	if err := s.sendVehiclePackets(c, packets); err != nil {
		return err
	}
	return c.send([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
}

// wearVehicle follows Inventory.ApplyVehicleWear and AC06's authored beach wreck.
// The reference has no active fuel consumption path. Only moved rafts wear.
func (s *Server) wearVehicle(ctx context.Context, c *Session) error {
	old := *c.character
	if old.ActiveVehicle == 0 {
		return nil
	}
	item, ok := old.Vehicle(old.VehicleSlot, old.ActiveVehicle, s.Assets.Items)
	if !ok {
		return s.dismountVehicle(ctx, c)
	}
	if !game.Raft(item.ID) {
		return nil
	}
	if item.Damage >= game.VehicleWreckDamage-1 || (item.ID == game.DisposableRaftItemID && old.Map == game.MapID11016 && old.X >= 280 && old.Y >= 950) {
		return s.wreckVehicle(ctx, c)
	}
	next := old.Clone()
	next.Bag[old.VehicleSlot-1].Damage++
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	return c.send(next.Bag.Packet(protocol.CommandInventory, protocol.InventoryItems))
}
