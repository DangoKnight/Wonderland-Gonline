package server

import (
	"context"
	"errors"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

var errBreillatUnavailable = errors.New("Breillat conversion is unavailable")

// The acceptance branch's marks and model are committed together. Read fresh
// player state inside the transaction, and keep every item and progression field.
func (s *Server) convertBreillat(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	if !op.BreillatTransform() || !world.IsBreillat(es.ev) || es.ev.Branches[es.branch].Index != world.BreillatAcceptanceBranch {
		return false, nil
	}
	var next game.Character
	now := time.Now().UTC()
	err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error {
		if err := game.PreserveItemLocks(*c.character, stored); err != nil {
			return err
		}
		talk, converted := stored.Quests[game.BreillatTalkMark], stored.Quests[game.BreillatConversionMark]
		if stored.Map != es.mapID || talk.State != game.InProgress || talk.Step < game.BreillatRequiredTalks || converted.State == game.Completed || converted.Step > 0 {
			return errBreillatUnavailable
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
	if err != nil {
		return false, errors.Join(err, s.cancelInteraction(c))
	}
	*c.character = next
	appearance, err := next.AppearancePacket(true)
	if err != nil {
		return false, err
	}
	model := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateModelTransform}.U32(next.ID).U8(game.BreillatModel)
	s.broadcastWorld(c, appearance)
	s.broadcastWorld(c, model)
	packets := [][]byte{s.World.HideActor(c.view, es.mapID, world.BreillatActor), model}
	packets = append(packets, next.StatPackets(s.Assets.Items)...)
	for _, mark := range []uint32{game.BreillatTalkMark, game.BreillatConversionMark} {
		packets = append(packets, s.World.QuestUpdate(c.view, mark, next.Quests[mark])...)
	}
	packets = append(packets, []byte{protocol.CommandEvent, protocol.EventStepComplete})
	return true, s.sendAll(c, packets)
}
