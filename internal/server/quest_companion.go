package server

import (
	"context"
	"errors"
	"slices"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func (s *Server) canChangeQuestCompanion(c *game.Character, plan world.QuestCompanionPlan) bool {
	_, present := c.Pet(plan.Template)
	if !plan.Join {
		return present
	}
	// Reissue must not produce a second copy while an escort is stored elsewhere.
	return !present && s.canRecruit(c, plan.Template) && !slices.ContainsFunc(c.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, plan.Template) })
}

func (s *Server) changeQuestCompanion(ctx context.Context, c *Session, es *eventSession, plan world.QuestCompanionPlan) error {
	before := c.character.Clone()
	var pet game.Pet
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: before.ID}, func(current *game.Character) error {
		if c.event != es || current.Map != es.mapID || s.World.FindBranch(current, c.view, es.mapID, es.ev, world.TriggerEntry, 0, 0, -1) != es.branch || !s.canChangeQuestCompanion(current, plan) {
			return errQuestCompletionUnavailable
		}
		if a, b := current.Quests[plan.Mark], before.Quests[plan.Mark]; a.State != b.State || a.Step != b.Step {
			return errQuestCompletionUnavailable
		}
		if plan.Join {
			template := s.petTemplate(plan.Template)
			pet = game.NewPet(plan.Template, s.companionName(plan.Template, ""), current.FreePetSlot(), template, s.Assets.Items)
			pet.EnsureSkills(template, s.hasSkill)
			pet.Normalize(s.Assets.Items, true, template)
			current.Pets = append(current.Pets, pet)
		} else {
			i, _ := current.Pet(plan.Template)
			pet = current.Pets[i]
			current.Pets = slices.Delete(current.Pets, i, i+1)
			if game.SamePet(current.ActivePet, pet.ID) {
				current.ActivePet = 0
			}
			if game.SamePet(current.ActiveMount, pet.ID) {
				current.ActiveMount = 0
			}
		}
		return applyStoryMarks(current, plan.Marks, time.Now().UTC())
	}, before)
	if err != nil {
		return errors.Join(err, s.cancelInteraction(c))
	}
	s.adoptSavedCharacter(c, next)
	es.index = len(es.ev.Branches[es.branch].Operations)
	var packets [][]byte
	if plan.Join {
		if c.pets.register(pet.ID) {
			packets = append(packets, pet.RecruitPacket(next.ID, s.petTemplate(pet.ID)))
		}
		slot := c.pets.slot(pet.ID)
		packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))...)
		packets = append(packets, serverwire.PetNamePacket(next.ID, slot, s.companionName(pet.ID, pet.Name)))
		if p := s.petListPacket(c.character, c.pets); p != nil {
			packets = append(packets, p)
		}
	} else {
		if before.ActiveMount != next.ActiveMount {
			for _, p := range serverwire.UnmountPackets(next.ID) {
				packets = append(packets, p)
				s.broadcastWorld(c, p)
			}
		}
		if before.ActivePet != next.ActivePet {
			packets = append(packets, []byte{protocol.CommandBattlePet, protocol.BattlePetRest})
			s.broadcastWorld(c, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(next.ID))
			if next.ActiveMount == 0 {
				p := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U32(0)
				packets = append(packets, p)
				s.broadcastWorld(c, p)
			}
		}
		p := protocol.Builder{protocol.CommandPetControl, protocol.PetControlPetSlot}.U32(next.ID).U8(c.pets.release(pet.ID))
		packets = append(packets, p)
		s.broadcastWorld(c, p)
	}
	for _, mark := range plan.Marks {
		packets = append(packets, s.questMarkPackets(c, mark.ID, next.Quests[mark.ID])...)
	}
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	for _, op := range plan.Actors {
		if _, err := s.actorAction(ctx, c, es, op); err != nil {
			return err
		}
	}
	return nil
}
