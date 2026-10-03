package server

import (
	"context"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// mountPacket is PutPetToRide / SendPeerCompanionAndVehicle: slot, owner,
// companion ID, and 26 reserved bytes. Peer snapshots use slot one as in C#.
func mountPacket(owner, id uint32, slot byte) []byte {
	return protocol.Builder{protocol.CommandPetControl, protocol.PetControlMount, slot}.U32(owner).U32(id).Bytes(make([]byte, 26))
}

func unmountPackets(owner uint32) [][]byte {
	return [][]byte{protocol.Builder{protocol.CommandPetControl, protocol.PetControlUnmount}.U32(owner), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(owner).U8(0)}
}

func mountedPet(c *game.Character) *game.Pet {
	if c.ActiveMount == 0 {
		return nil
	}
	if i, ok := c.Pet(c.ActiveMount); ok {
		return &c.Pets[i]
	}
	return nil
}

// mountCommand is AC15:11/12. Caller holds worldMu and has checked readiness and
// battle state. Unlike the legacy handler, stale client slot/ID pairs are ignored.
func (s *Server) mountCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if p[1] != 11 && p[1] != 12 {
		return ErrUnsupported
	}
	if len(p) != 7 {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	slot, id := r.U8(), r.U32()
	if r.Err() != nil {
		return r.Err()
	}
	i, ok := c.character.Pet(id)
	if !ok || slot == 0 || c.pets.slot(c.character.Pets[i].ID) != slot {
		return nil
	}
	pet := c.character.Pets[i]
	next := c.character.Clone()
	var packets [][]byte
	if p[1] == 11 {
		next.ActiveVehicle, next.VehicleSlot = 0, 0
		next.ActiveMount = game.BroadcastID(pet.ID)
		if next.ActiveMount == c.character.ActiveMount {
			return nil
		}
		packets = [][]byte{mountPacket(next.ID, next.ActiveMount, pet.Slot), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U8(0)}
	} else {
		if next.ActiveMount == 0 || !game.SamePet(next.ActiveMount, pet.ID) {
			return nil
		}
		next.ActiveMount = 0
		packets = unmountPackets(next.ID)
	}
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	for _, packet := range packets {
		s.broadcastWorld(c, packet)
	}
	return s.sendAll(c, packets)
}
