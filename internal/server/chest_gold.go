package server

import (
	"context"
	"errors"
	"fmt"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func (s *Server) claimQuestChestGold(ctx context.Context, c *Session, es *eventSession, pool assets.ChestPool, gold uint32) (bool, error) {
	if c.event != es {
		return false, nil
	}
	now := time.Now().UTC()
	key := world.ChestKey(es.mapID, es.ev.ClickID)
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		// The accepted script may already have changed its presentation/marks.
		// Recheck durable map and cooldown rather than replaying entry conditions.
		if next.Map != es.mapID || now.Before(next.ChestRespawns[key]) {
			return store.ErrPropEmpty
		}
		if gold == 0 || uint64(next.Gold)+uint64(gold) > game.MaxGold {
			return store.ErrPropRewardUnavailable
		}
		next.Gold += gold
		if next.ChestRespawns == nil {
			next.ChestRespawns = map[uint32]time.Time{}
		}
		next.ChestRespawns[key] = now.Add(time.Duration(pool.RespawnSeconds) * time.Second)
		if next.Quests == nil {
			next.Quests = map[uint32]game.Quest{}
		}
		next.Quests[key] = game.Quest{ID: key, State: game.Completed, Step: 1, StartedAt: now}
		return nil
	}, *c.character)
	if errors.Is(err, store.ErrPropEmpty) {
		return true, nil
	}
	if errors.Is(err, store.ErrPropRewardUnavailable) {
		return false, c.send(headBanner(err.Error()))
	}
	if err != nil {
		return false, err
	}
	s.adoptSavedCharacter(c, next)
	if c.view != nil {
		c.view.Props[es.ev.ClickID] = propBrokenFrame
	}
	return true, s.sendAll(c, [][]byte{
		protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold),
		headBanner(fmt.Sprintf("Obtain %d gold", gold)),
		{protocol.CommandEvent, protocol.EventStepComplete},
		propFrame(es.ev.ClickID, propBrokenFrame),
	})
}
