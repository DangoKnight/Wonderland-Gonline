package server

import (
	"context"
	"errors"
	"slices"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// companionRebirthCommand is the native altar operation. Opening its authored
// service authorizes one successful request during this map visit. Commit before
// replying and keep the form usable after a rejected selection or failed save.
func (s *Server) companionRebirthCommand(ctx context.Context, c *Session, slot byte) error {
	reply := func(code byte) error {
		return c.send([]byte{protocol.CommandCompanionRebirth, protocol.CompanionRebirthRequest, code})
	}
	if c.view == nil || !c.view.CompanionRebirthOpen || c.event != nil || c.storm || c.beach != nil {
		return reply(protocol.CompanionRebirthUnavailable)
	}
	if c.pets == nil || slot == 0 {
		return reply(protocol.CompanionRebirthNoPet)
	}
	i := slices.IndexFunc(c.character.Pets, func(p game.Pet) bool { return p.ID != 0 && c.pets.slot(p.ID) == slot })
	if i < 0 {
		return reply(protocol.CompanionRebirthNoPet)
	}
	next := c.character.Clone()
	p := &next.Pets[i]
	if p.Reborn {
		return reply(protocol.CompanionRebirthAlready)
	}
	if p.Level < game.PetRebirthRequiredLevel {
		return reply(protocol.CompanionRebirthLevel)
	}
	if p.Amity < game.CompanionRebirthRequiredAmity {
		return reply(protocol.CompanionRebirthAmity)
	}
	for _, item := range p.Equipment {
		if item.ID != 0 {
			return reply(protocol.CompanionRebirthEquipment)
		}
	}
	if !p.RebirthAtAltar(s.Assets.Items) {
		return reply(protocol.CompanionRebirthUnavailable)
	}
	id := p.ID
	denied := errors.New("companion rebirth eligibility changed")
	code := byte(protocol.CompanionRebirthUnavailable)
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(stored *game.Character) error {
		index := slices.IndexFunc(stored.Pets, func(p game.Pet) bool { return p.ID == id })
		if index < 0 {
			code = protocol.CompanionRebirthNoPet
			return denied
		}
		pet := &stored.Pets[index]
		if pet.Reborn {
			code = protocol.CompanionRebirthAlready
			return denied
		}
		if pet.Level < game.PetRebirthRequiredLevel {
			code = protocol.CompanionRebirthLevel
			return denied
		}
		if pet.Amity < game.CompanionRebirthRequiredAmity {
			code = protocol.CompanionRebirthAmity
			return denied
		}
		for _, item := range pet.Equipment {
			if item.ID != 0 {
				code = protocol.CompanionRebirthEquipment
				return denied
			}
		}
		if !pet.RebirthAtAltar(s.Assets.Items) {
			return denied
		}
		i = index
		return nil
	}, *c.character)
	if errors.Is(err, denied) {
		return reply(code)
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	p = &next.Pets[i]
	c.view.CompanionRebirthOpen = false
	packets := [][]byte{{protocol.CommandCompanionRebirth, protocol.CompanionRebirthRequest, protocol.CompanionRebirthSuccess}, s.petListPacket(c.character, c.pets)}
	packets = append(packets, p.ProgressionPackets(slot, s.Assets.Items)...)
	return s.sendAll(c, packets)
}
