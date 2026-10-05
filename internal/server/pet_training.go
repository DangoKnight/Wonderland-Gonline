package server

import (
	"context"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// petTrainingCommand ports AC68:2. Caller holds worldMu and enforces world
// loading/battle/trade gates. Potential pills use the separate native AC23:126 handler.
func (s *Server) petTrainingCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.PetTrainingRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.PetTrainingAllocate && p[1] != protocol.PetTrainingPotential {
		return ErrUnsupported
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	slot, selector := p[2], p[3]
	reply := []byte{protocol.CommandPetTraining, p[1], slot, selector, protocol.PetTrainingFailed}
	if c.pets == nil {
		return c.send(reply)
	}
	i := s.clientPet(c, slot)
	if i < 0 || c.character.Pets[i].ID == 0 {
		return c.send(reply)
	}
	if p[1] == protocol.PetTrainingPotential {
		return s.sendAll(c, [][]byte{reply, systemLine("Use a Potential Pill from inventory; free potential training is disabled.")})
	}
	var stat byte
	switch selector {
	case protocol.PetTrainingStrength:
		stat = game.StatSTR
	case protocol.PetTrainingConstitution:
		stat = game.StatCON
	case protocol.PetTrainingIntelligence:
		stat = game.StatINT
	case protocol.PetTrainingWisdom:
		stat = game.StatWIS
	case protocol.PetTrainingAgility:
		stat = game.StatAGI
	default:
		return c.send(reply)
	}
	next := c.character.Clone()
	pet := &next.Pets[i]
	if !pet.AllocatePoint(stat) {
		return c.send(reply)
	}
	pet.Normalize(s.Assets.Items, false)
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	packets := pet.ProgressionPackets(slot, s.Assets.Items)
	reply[4] = protocol.PetTrainingSucceeded
	return s.sendAll(c, append(packets, reply))
}
