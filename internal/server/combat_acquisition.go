package server

import (
	"context"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func (s *Server) commitCombatCapture(run *battleRun, catch battle.Capture) (battle.Step, bool) {
	for _, m := range run.active() {
		if m.self.ID != catch.Owner {
			continue
		}
		c := m.c
		var saved game.Character
		var pet game.Pet
		t := s.petTemplate(catch.Template)
		ctx, cancel := context.WithTimeout(context.Background(), autosaveTimeout)
		defer cancel()
		err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(next *game.Character) error {
			slot := next.FreePetSlot()
			if _, exists := next.Pet(catch.Template); exists || slot == 0 {
				return game.ErrInvalidItem
			}
			pet = game.NewCapturedPet(catch.Template, catch.Name, slot, t, s.Assets.Items, petGrowth, game.PetGrowthOptions{Formula: s.petGrowthFormula, Items: s.Assets.Items})
			pet.EnsureSkills(t, s.hasSkill)
			next.Pets = append(next.Pets, pet)
			if err := game.PreserveItemLocks(*c.character, next); err != nil {
				return err
			}
			saved = next.Clone()
			return nil
		})
		if err != nil {
			run.rewardError = err
			return battle.Step{}, false
		}
		s.adoptSavedCharacter(c, saved)
		c.pets.register(pet.ID)
		packets := [][]byte{pet.RecruitPacket(catch.Owner, t)}
		if pet.Equipment != (game.Equipment{}) {
			packets = append(packets, s.petListPacket(c.character, c.pets))
		}
		return battle.Step{OwnerPackets: map[uint32][][]byte{catch.Owner: packets}}, true
	}
	return battle.Step{}, false
}

func (s *Server) commitCombatLoot(run *battleRun, target *battle.Fighter, actors []battle.Action) battle.Step {
	step := battle.Step{OwnerPackets: map[uint32][][]byte{}}
	if run.b.PvP || target.Kind != battle.Monster || target.Captured || !target.Dead() || run.rewardError != nil {
		return step
	}
	if run.rewarded == nil {
		run.rewarded = map[*battle.Fighter]bool{}
	}
	if run.rewarded[target] {
		return step
	}
	run.rewarded[target] = true
	r := s.rules()
	r.DropRateMultiplier = s.dropRateMultiplier
	npc := s.Assets.NPCs[uint16(target.Template)]
	drops := r.RollDrops(s.Assets.Drops[target.Template], npc.Drops, func(id uint16) bool { _, ok := s.Assets.Items[id]; return ok })
	if len(drops) == 0 {
		return step
	}
	type recipient struct {
		c      *Session
		actors []*battle.Fighter
	}
	var recipients []recipient
	indices := map[uint32]int{}
	seen := map[*battle.Fighter]bool{}
	for _, a := range actors {
		if a.Actor.Kind != battle.Player && a.Actor.Kind != battle.Pet {
			continue
		}
		if seen[a.Actor] {
			continue
		}
		seen[a.Actor] = true
		owner := a.Actor.ID
		if a.Actor.Kind == battle.Pet {
			owner = a.Actor.Owner
		}
		for _, m := range run.active() {
			if !s.trainingRewardEligible(m.c, run) {
				continue
			}
			if m.self.ID != owner {
				continue
			}
			i, ok := indices[owner]
			if !ok {
				i = len(recipients)
				indices[owner] = i
				recipients = append(recipients, recipient{c: m.c})
			}
			recipients[i].actors = append(recipients[i].actors, a.Actor)
		}
	}
	if len(recipients) == 0 {
		return step
	}
	refs := make([]store.CharacterRef, len(recipients))
	for i, recipient := range recipients {
		refs[i] = store.CharacterRef{Account: recipient.c.account.ID, ID: recipient.c.character.ID}
	}
	var saved []game.Character
	flights := make([]protocol.Builder, len(drops))
	for i, drop := range drops {
		flights[i] = protocol.Builder{protocol.CommandBattleEffect, protocol.BattleEffectItemGain}.U16(drop.Item).U8(target.X).U8(target.Y)
	}
	ctx, cancel := context.WithTimeout(context.Background(), autosaveTimeout)
	defer cancel()
	err := s.Store.UpdateCharacters(ctx, refs, func(chars []game.Character) error {
		for i := range chars {
			c := &chars[i]
			if err := game.PreserveItemLocks(*recipients[i].c.character, c); err != nil {
				return err
			}
			for d, drop := range drops {
				for _, actor := range recipients[i].actors {
					item := game.Item{ID: drop.Item, Count: 1}
					limit, _ := s.stackLimit(item.ID)
					if _, err := c.Bag.Grant(item, int(item.Count), limit, s.Assets.Items); err != nil {
						continue
					}
					step.OwnerPackets[c.ID] = append(step.OwnerPackets[c.ID], game.AcquisitionPacket(item))
					flights[d] = flights[d].U8(actor.X).U8(actor.Y)
				}
			}
		}
		saved = chars
		return nil
	})
	if err != nil {
		run.rewardError = err
		return battle.Step{}
	}
	for i, recipient := range recipients {
		s.adoptSavedCharacter(recipient.c, saved[i])
	}
	for _, m := range run.active() {
		for _, fx := range flights {
			if len(fx) > 6 {
				step.OwnerPackets[m.self.ID] = append(step.OwnerPackets[m.self.ID], fx)
			}
		}
	}
	return step
}
