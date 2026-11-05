package server

import (
	"math"
	"slices"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

type questCompanionMutation struct {
	pet                                            game.Pet
	recruit, first, changed, wasActive, wasMounted bool
}

// This stages the ordinary recruit/dismiss contract on the transaction's owned
// character. No client roster or actor visibility changes before SQL commits.
func (s *Server) stageQuestCompanion(current *game.Character, reward world.QuestCompanionReward) (questCompanionMutation, error) {
	change := questCompanionMutation{recruit: reward.Recruit, first: true}
	id := reward.ID
	if alias, known := s.Assets.Story.CompanionAliases[id]; known {
		id = alias
	}
	i, present := current.Pet(id)
	if reward.Recruit {
		if s.recruitBlocked(current, id) || !s.canRecruit(current, id) {
			return change, errQuestCompletionUnavailable
		}
		if present {
			return change, nil
		}
		slot := current.FreePetSlot()
		if slot == 0 {
			return change, errQuestCompletionUnavailable
		}
		if reserve := slices.IndexFunc(current.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, id) }); reserve >= 0 {
			change.pet, change.first = current.ReservePets[reserve], false
			current.ReservePets = slices.Delete(current.ReservePets, reserve, reserve+1)
			change.pet.Slot = slot
		} else {
			template := s.petTemplate(id)
			change.pet = game.NewPet(id, s.companionName(id, ""), slot, template, s.Assets.Items)
			change.pet.EnsureSkills(template, s.hasSkill)
			change.pet.Normalize(s.Assets.Items, true, template)
		}
		current.Pets = append(current.Pets, change.pet)
	} else {
		if !present {
			return change, nil
		}
		change.pet = current.Pets[i]
		template, known := s.World.Template(game.BroadcastID(change.pet.ID))
		if game.StoryCompanion(change.pet.ID, template, known) {
			if slices.ContainsFunc(current.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, change.pet.ID) }) {
				return change, errQuestCompletionUnavailable
			}
			reserve := byte(1)
			for slices.ContainsFunc(current.ReservePets, func(p game.Pet) bool { return p.Slot == reserve }) {
				if reserve == math.MaxUint8 {
					return change, errQuestCompletionUnavailable
				}
				reserve++
			}
			kept := change.pet
			kept.Slot, kept.Battle = reserve, false
			current.ReservePets = append(current.ReservePets, kept)
		}
		change.wasActive = current.ActivePet != 0 && game.SamePet(current.ActivePet, change.pet.ID)
		change.wasMounted = current.ActiveMount != 0 && game.SamePet(current.ActiveMount, change.pet.ID)
		if change.wasActive {
			current.ActivePet = 0
		}
		if change.wasMounted {
			current.ActiveMount = 0
		}
		current.Pets = slices.Delete(current.Pets, i, i+1)
	}
	change.changed = true
	return change, nil
}

func (s *Server) questCompanionPackets(c *Session, changes []questCompanionMutation) [][]byte {
	var packets [][]byte
	for _, change := range changes {
		if !change.changed {
			continue
		}
		pet := change.pet
		if change.recruit {
			if m, known := s.World.Map(c.character.Map); known {
				for _, n := range m.NPCs {
					if n.Template > 0 && s.World.InParty(c.character, n.Template) {
						packets = append(packets, s.World.HideActor(c.view, c.character.Map, n.ClickID))
					}
				}
			}
			if c.pets.register(pet.ID) {
				packets = append(packets, pet.RecruitPacket(c.character.ID, s.petTemplate(pet.ID)))
			}
			slot := c.pets.slot(pet.ID)
			packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))...)
			packets = append(packets, serverwire.PetNamePacket(c.character.ID, slot, s.companionName(pet.ID, pet.Name)))
			if !change.first || pet.Equipment != (game.Equipment{}) {
				if p := s.petListPacket(c.character, c.pets); p != nil {
					packets = append(packets, p)
				}
			}
			continue
		}
		if change.wasMounted {
			for _, p := range serverwire.UnmountPackets(c.character.ID) {
				packets = append(packets, p)
				s.broadcastWorld(c, p)
			}
		}
		if change.wasActive {
			packets = append(packets, []byte{protocol.CommandBattlePet, protocol.BattlePetRest})
			s.broadcastWorld(c, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(c.character.ID))
			if c.character.ActiveMount == 0 {
				p := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(c.character.ID).U32(0)
				packets = append(packets, p)
				s.broadcastWorld(c, p)
			}
		}
		p := protocol.Builder{protocol.CommandPetControl, protocol.PetControlPetSlot}.U32(c.character.ID).U8(c.pets.release(pet.ID))
		packets = append(packets, p)
		s.broadcastWorld(c, p)
	}
	return packets
}
