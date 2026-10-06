package server

import (
	"context"
	"wonderland-gonline/internal/protocol"
)

const (
	petFeedingItemsPerRequest   = 1
	petFeedingAmityCap          = 100
	petFeedingAmityStatus       = 64
	petFeedingNativeValueOffset = 100
)

// petFeedingCommand ports AC67 and PetAmityManager.FeedPet. The request names
// an item ID, not a bag slot. Only a registered party pet marked for battle is
// eligible; TryFeedPet's shared consumption policy owns the durable transition.
func (s *Server) petFeedingCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.PetFeedingRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.PetFeedingFeed {
		return ErrUnsupported
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	r := protocol.NewReader(p[2:])
	food := r.U16()
	reply := protocol.Builder{protocol.CommandPetFeeding, protocol.PetFeedingFeed}.U16(food)
	var target byte
	if c.pets != nil {
		for _, pet := range c.character.Pets {
			if pet.ID != 0 && pet.Battle && c.pets.slot(pet.ID) != 0 {
				target = c.pets.slot(pet.ID)
				break
			}
		}
	}
	if target != 0 && food != 0 {
		for index, item := range c.character.Bag {
			if item.ID != food || item.Empty() {
				continue
			}
			ok, err := s.feedPet(ctx, c, byte(index+1), petFeedingItemsPerRequest, target)
			if err != nil {
				return err
			}
			if ok {
				return c.send(reply.U8(protocol.PetFeedingSucceeded))
			}
		}
	}
	return s.sendAll(c, [][]byte{headBanner("Select a battle pet and use an available amity item."), reply.U8(protocol.PetFeedingFailed)})
}
