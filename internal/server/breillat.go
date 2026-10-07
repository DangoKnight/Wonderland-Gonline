package server

import (
	"context"
	"errors"
	"fmt"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

var errBreillatUnavailable = errors.New("Breillat conversion is unavailable")

var errBreillatExchangeUnavailable = errors.New("Breillat exchange is unavailable")

func (s *Server) redeemBreillat(ctx context.Context, c *Session, es *eventSession, changes []game.ItemChange) error {
	var result game.QuestItemResult
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(stored *game.Character) error {
		if stored.Map != es.mapID || s.World.FindBranch(stored, c.view, es.mapID, es.ev, world.TriggerEntry, 0, 0, -1) != es.branch {
			return errBreillatExchangeUnavailable
		}
		var err error
		result, err = stored.Bag.ApplyQuestItems(changes, s.stackLimit, s.Assets.Items)
		if err != nil {
			return fmt.Errorf("%w: %v", errBreillatExchangeUnavailable, err)
		}
		return nil
	}, *c.character)
	if err != nil {
		if !errors.Is(err, errBreillatExchangeUnavailable) {
			return errors.Join(err, s.cancelInteraction(c))
		}
		return c.send(headBanner("Cannot exchange vouchers. Check materials and inventory space."))
	}
	s.adoptSavedCharacter(c, next)
	es.index = len(es.ev.Branches[es.branch].Operations)
	var packets [][]byte
	for _, removed := range result.Removed {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, removed.Slot, removed.Count})
	}
	packets = append(packets, next.Bag.AdditionPacket(result.Added))
	for _, change := range changes {
		verb, count := "Obtain", change.Count
		if count < 0 {
			verb, count = "Lost", -count
		}
		packets = append(packets, headBanner(fmt.Sprintf("%s %s x%d", verb, s.itemName(change.ID), count)))
	}
	packets = append(packets, []byte{protocol.CommandEvent, protocol.EventStepComplete})
	return s.sendAll(c, packets)
}

// The acceptance branch's marks and model are committed together. Read fresh
// player state inside the transaction, including the outfit replacement.
func (s *Server) convertBreillat(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	if !op.BreillatTransform() || !world.IsBreillat(es.ev) || es.ev.Branches[es.branch].Index != world.BreillatAcceptanceBranch {
		return false, nil
	}
	var next game.Character
	var beforeBag game.Inventory
	now := time.Now().UTC()
	err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error {
		if err := game.PreserveItemLocks(*c.character, stored); err != nil {
			return err
		}
		talk, converted := stored.Quests[game.BreillatTalkMark], stored.Quests[game.BreillatConversionMark]
		if stored.Map != es.mapID || talk.State != game.InProgress || talk.Step < game.BreillatRequiredTalks || converted.State == game.Completed || converted.Step > 0 {
			return errBreillatUnavailable
		}
		beforeBag = stored.Bag
		if err := stored.ReplaceBreillatOutfit(s.Assets.Items); err != nil {
			return err
		}
		stored.Body, stored.Head = game.BreillatBody, game.BreillatHead
		talk.State, talk.Step, talk.CompletedAt = game.Completed, 0, &now
		stored.Quests[game.BreillatTalkMark] = talk
		stored.Quests[game.BreillatConversionMark] = game.Quest{ID: game.BreillatConversionMark, State: game.InProgress, Step: 1, StartedAt: now}
		full := stored.Combat(s.Assets.Items)
		stored.MaxHP, stored.MaxSP = uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
		stored.HP, stored.SP = min(stored.HP, stored.MaxHP), min(stored.SP, stored.MaxSP)
		next = stored.Clone()
		return nil
	})
	if errors.Is(err, errBreillatUnavailable) {
		return false, c.send(headBanner("Breillat's transformation is no longer available."))
	}
	if errors.Is(err, game.ErrInventoryFull) || errors.Is(err, game.ErrItemLocked) {
		return false, c.send(headBanner("Cannot transform. Free inventory space and release reserved equipment."))
	}
	if err != nil {
		return false, errors.Join(err, s.cancelInteraction(c))
	}
	s.adoptSavedCharacter(c, next)
	appearance, err := next.AppearancePacket(true)
	if err != nil {
		return false, err
	}
	model := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateModelTransform}.U32(next.ID).U8(game.BreillatModel)
	s.broadcastWorld(c, appearance)
	s.broadcastWorld(c, model)
	packets := [][]byte{s.World.HideActor(c.view, es.mapID, world.BreillatActor), model}
	var additions []game.Addition
	for i, previous := range beforeBag {
		if previous == next.Bag[i] {
			continue
		}
		if !previous.Empty() {
			packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, byte(i + 1), previous.Count})
		}
		if !next.Bag[i].Empty() {
			additions = append(additions, game.Addition{Slot: byte(i + 1), Count: next.Bag[i].Count})
		}
	}
	if len(additions) > 0 {
		packets = append(packets, next.Bag.AdditionPacket(additions))
	}
	packets = append(packets, next.EquipmentPacket())
	packets = append(packets, next.StatPackets(s.Assets.Items)...)
	for _, mark := range []uint32{game.BreillatTalkMark, game.BreillatConversionMark} {
		packets = append(packets, s.World.QuestUpdate(c.view, mark, next.Quests[mark])...)
	}
	packets = append(packets, []byte{protocol.CommandEvent, protocol.EventStepComplete})
	return true, s.sendAll(c, packets)
}
