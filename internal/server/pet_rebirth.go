package server

import (
	"context"
	"fmt"
	"math"
	"slices"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// petRebirthCommand ports AC69. The dispatcher holds worldMu and guards loading,
// battle and trade. Ascension is selected by the session's native client slot.
func (s *Server) petRebirthCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.PetRebirthRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.PetRebirthAscend {
		return ErrUnsupported
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	slot := p[2]
	reply := []byte{protocol.CommandPetRebirth, protocol.PetRebirthAscend, slot, protocol.PetRebirthFailed}
	if c.pets == nil || slot == 0 {
		return c.send(reply)
	}
	i := slices.IndexFunc(c.character.Pets, func(pet game.Pet) bool { return pet.ID != 0 && c.pets.slot(pet.ID) == slot })
	if i < 0 {
		return c.send(reply)
	}
	next := c.character.Clone()
	pet := &next.Pets[i]
	if !pet.Rebirth(s.Assets.Items) {
		message := fmt.Sprintf("[Rebirth] %s requires level %d and amity %d.", pet.Name, game.PetRebirthRequiredLevel, game.PetRebirthRequiredAmity)
		if pet.Reborn {
			message = fmt.Sprintf("[Rebirth] %s has already undergone Rebirth ascension.", pet.Name)
		} else if pet.StatPoints > math.MaxUint16-game.PetRebirthBonusStatPoints {
			message = "[Rebirth] Spend some pet stat points before receiving the rebirth bonus."
		}
		return s.sendAll(c, [][]byte{reply, systemLine(message)})
	}
	// Normalize before saving so reconnect sees the exact same full vitals.
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	reply[3] = protocol.PetRebirthSucceeded
	packets := [][]byte{reply, s.petListPacket(c.character, c.pets)}
	packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items)...)
	packets = append(packets, systemLine(fmt.Sprintf("[Rebirth] %s has attained Rebirth Ascension!", pet.Name)))
	return s.sendAll(c, packets)
}
