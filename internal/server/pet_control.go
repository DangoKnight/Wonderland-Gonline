package server

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func (s *Server) petControlCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.PetControlBoardVehicle, protocol.PetControlBoardVehicleAlternate, protocol.PetControlDismountVehicle, protocol.PetControlVehicleNoOp, protocol.PetControlPlaceVehicle:
		return s.vehicleCommand(ctx, c, p)
	case protocol.PetControlPetSlot:
		return s.releasePet(ctx, c, p)
	case protocol.PetControlRename:
		return s.renamePet(ctx, c, p)
	default:
		return s.mountCommand(ctx, c, p)
	}
}

// renamePet is AC15:6: client slot followed by raw ASCII bytes (not a
// length-prefixed string). Match .NET ASCII replacement, trimming and 16-byte cap.
func (s *Server) renamePet(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 4 {
		return protocol.ErrMalformed
	}
	slot := p[2]
	i := slices.IndexFunc(c.character.Pets, func(p game.Pet) bool { return slot != 0 && c.pets.slot(p.ID) == slot })
	if i < 0 {
		return nil
	}
	raw := append([]byte(nil), p[3:]...)
	for i, b := range raw {
		if b >= 128 {
			raw[i] = '?'
		}
	}
	name := strings.TrimSpace(strings.TrimRight(string(raw), "\x00"))
	name = name[:min(16, len(name))]
	if name == "" {
		return nil
	}
	for _, b := range []byte(name) {
		if b < 32 || b == 127 {
			return nil
		}
	}
	if name == c.character.Pets[i].Name {
		return nil
	}
	next := c.character.Clone()
	next.Pets[i].Name = name
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	pet := next.Pets[i]
	if pet.Battle || (next.ActivePet != 0 && game.SamePet(next.ActivePet, pet.ID)) {
		s.broadcastWorld(c, petMapPacket(next.ID, game.BroadcastID(pet.ID), name))
	}
	return c.send(petNamePacket(next.ID, slot, name))
}

// releasePet permanently releases the client-selected party pet. EVE dismiss
// remains separate: story companions temporarily dismissed there enter reserve.
func (s *Server) releasePet(ctx context.Context, c *Session, p []byte) error {
	if len(p) != 3 {
		return protocol.ErrMalformed
	}
	slot := p[2]
	i := slices.IndexFunc(c.character.Pets, func(p game.Pet) bool { return slot != 0 && c.pets.slot(p.ID) == slot })
	if i < 0 {
		return nil
	}
	next := c.character.Clone()
	pet := next.Pets[i]
	var packets, peers [][]byte
	if next.ActiveMount != 0 && game.SamePet(next.ActiveMount, pet.ID) {
		next.ActiveMount = 0
		packets = append(packets, unmountPackets(next.ID)...)
		peers = append(peers, unmountPackets(next.ID)...)
	}
	if pet.Battle || (next.ActivePet != 0 && game.SamePet(next.ActivePet, pet.ID)) {
		next.ActivePet = 0
		packets = append(packets, []byte{protocol.CommandBattlePet, protocol.BattlePetRest})
		peers = append(peers, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(next.ID))
		if next.ActiveMount == 0 {
			despawn := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U32(0)
			packets = append(packets, despawn)
			peers = append(peers, despawn)
		}
	}
	next.Pets = slices.Delete(next.Pets, i, i+1)
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	c.pets.release(pet.ID)
	removed := protocol.Builder{protocol.CommandPetControl, protocol.PetControlPetSlot}.U32(next.ID).U8(slot)
	packets = append(packets, removed)
	peers = append(peers, removed)
	for _, packet := range peers {
		s.broadcastWorld(c, packet)
	}
	packets = append(packets, systemLine(fmt.Sprintf("Released companion pet %s (Slot %d).", pet.Name, slot)))
	return s.sendAll(c, packets)
}
