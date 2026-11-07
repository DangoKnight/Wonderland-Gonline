package server

import (
	"context"
	"errors"
	"fmt"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

var errQuestCompletionUnavailable = errors.New("quest completion conditions changed")

func (s *Server) completeNativeQuest(ctx context.Context, c *Session, es *eventSession, plan world.QuestCompletionPlan) error {
	var result game.QuestItemResult
	var companionChanges []questCompanionMutation
	var skillPackets [][]byte
	leveled := false
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(current *game.Character) error {
		companionChanges, skillPackets = nil, nil
		leveled = false
		if c.event != es || current.Map != es.mapID || s.World.FindBranch(current, c.view, es.mapID, es.ev, world.TriggerEntry, 0, 0, -1) != es.branch {
			return errQuestCompletionUnavailable
		}
		for _, mark := range plan.Marks {
			expected, owned := c.character.Quests[mark.ID]
			actual, exists := current.Quests[mark.ID]
			if owned != exists || expected.State != actual.State || expected.Step != actual.Step {
				return errQuestCompletionUnavailable
			}
		}
		var err error
		result, err = current.Bag.ApplyQuestItems(plan.Items, s.stackLimit, s.Assets.Items)
		if err != nil {
			return err
		}
		for _, amount := range plan.Gold {
			balance := int64(current.Gold) + int64(amount)
			if balance < 0 || balance > game.MaxGold {
				return errQuestCompletionUnavailable
			}
			current.Gold = uint32(balance)
		}
		if plan.EXP != 0 {
			leveled = current.AddExp(plan.EXP) > 0
			if leveled {
				current.Refill(s.Assets.Items)
			}
		}
		for _, reward := range plan.Companions {
			change, err := s.stageQuestCompanion(current, reward)
			if err != nil {
				return err
			}
			companionChanges = append(companionChanges, change)
		}
		for _, id := range plan.Skills {
			skillPackets = append(skillPackets, current.LearnSkill(id, s.Assets.Skills[id].GradeLimit())...)
		}
		return applyStoryMarks(current, plan.Marks, time.Now().UTC())
	}, *c.character)
	if err != nil {
		return errors.Join(err, s.cancelInteraction(c))
	}
	s.adoptSavedCharacter(c, next)
	es.index = len(es.ev.Branches[es.branch].Operations)
	var packets [][]byte
	for _, removed := range result.Removed {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, removed.Slot, removed.Count})
	}
	if len(result.Added) != 0 {
		packets = append(packets, next.Bag.AdditionPacket(result.Added))
	}
	for _, change := range plan.Items {
		verb, count := "Obtain", change.Count
		if count < 0 {
			verb, count = "Lost", -count
		}
		packets = append(packets, serverwire.HeadBanner(fmt.Sprintf("%s %s x%d", verb, s.itemName(change.ID), count)))
	}
	for _, mark := range plan.Marks {
		packets = append(packets, s.questMarkPackets(c, mark.ID, next.Quests[mark.ID])...)
	}
	packets = append(packets, s.questCompanionPackets(c, companionChanges)...)
	packets = append(packets, skillPackets...)
	if len(plan.Gold) != 0 {
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
	}
	if plan.EXP != 0 {
		packets = append(packets, next.ExpPacket())
		if leveled {
			packets = append(packets, next.StatPackets(s.Assets.Items)...)
		}
	}
	packets = append(packets, []byte{protocol.CommandEvent, protocol.EventStepComplete})
	return s.sendAll(c, packets)
}
