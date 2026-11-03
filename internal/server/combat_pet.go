package server

import (
	"context"
	"encoding/binary"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
)

// battlePetCommand follows the Team command-phase gate in FUN_00277728.
// AC19 itself consumes the player's command; native clients send no AC50 pass.
// Caller holds worldMu. Save outgoing vitals and the selected mode atomically
// before changing the battle roster or announcing success.
func (s *Server) battlePetCommand(ctx context.Context, c *Session, p []byte) error {
	run := c.battle
	m := run.member(c)
	if m == nil || run.b.Processing || run.b.Finished || m.self.Dead() || !m.self.CanAct() || run.b.HasCommand(m.self) {
		return nil
	}
	var id uint32
	switch p[1] {
	case protocol.BattlePetSelect, protocol.BattlePetSelectAlternate:
		switch len(p) {
		case 3:
			for pet, slot := range c.pets.slots {
				if slot == p[2] {
					id = pet
				}
			}
		case 4:
			id = uint32(binary.LittleEndian.Uint16(p[2:]))
		case 6:
			id = binary.LittleEndian.Uint32(p[2:])
		default:
			return protocol.ErrMalformed
		}
		i, ok := c.character.Pet(id)
		if !ok {
			return nil
		}
		pet := c.character.Pets[i]
		if c.pets.slot(id) == 0 || pet.HP <= 0 || pet.Amity < game.BattlePetMinimumAmity || game.SamePet(c.character.ActiveMount, id) || m.pet != nil && game.SamePet(m.pet.ID, id) {
			return nil
		}
		id = pet.ID
	case protocol.BattlePetRest, protocol.BattlePetRestAlternate:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		if m.pet == nil {
			return nil
		}
	default:
		return nil
	}
	next := c.character.Clone()
	next.HP, next.SP = uint32(max(0, m.self.HP)), uint32(max(0, m.self.SP))
	roster := newPetRoster()
	roster.synced = c.pets.synced
	for pet, slot := range c.pets.slots {
		roster.slots[pet] = slot
	}
	packets, peers := s.activePetResults(run, m, &next, battle.Continue, roster)
	next.ActivePet = game.BroadcastID(id)
	for i := range next.Pets {
		next.Pets[i].Battle = id != 0 && game.SamePet(next.Pets[i].ID, id)
	}
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	c.pets = roster
	m.self.Char = c.character
	var incoming *battle.Fighter
	var selection []byte
	if id != 0 {
		i, _ := c.character.Pet(id)
		pet := c.character.Pets[i]
		incoming = battle.PetFighter(pet, next.ID, s.Assets.Items, 0)
		incoming.ClickID = uint16(c.pets.slot(id))
		selection = protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(next.ActivePet)
		peers = append(peers, serverwire.PetMapPacket(next.ID, next.ActivePet, s.companionName(pet.ID, pet.Name)))
	} else {
		selection = []byte{protocol.CommandBattlePet, protocol.BattlePetRest}
		peers = append(peers, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(next.ID))
	}
	changes := run.b.ReplacePet(m.self, m.pet, incoming)
	// Native order: owner submitted, old pet exit, private selection receipt,
	// incoming participant (mode 5). With no old pet, the receipt precedes entry.
	if m.pet == nil {
		changes = append([]battle.Step{{}}, changes...)
	}
	changes[0].OwnerPackets = map[uint32][][]byte{next.ID: {selection}}
	run.petChanges = append(run.petChanges, changes...)
	m.pet, m.choiceToken = incoming, 0
	for _, peer := range peers {
		s.broadcastWorld(c, peer)
	}
	s.sendOrClose(c, packets...)
	for _, member := range run.active() {
		s.sendOrClose(member.c, []byte{protocol.CommandBattleEffect, protocol.BattleEffectSubmitted, m.self.X, m.self.Y})
	}
	s.tryRound(run)
	return nil
}
