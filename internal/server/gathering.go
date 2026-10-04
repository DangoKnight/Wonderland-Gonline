package server

import (
	"context"
	"errors"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
	"wonderland-go/internal/world"
)

// Caller holds worldMu. The whole water branch is intercepted before the ordinary
// event loop so a failed timer claim cannot leave a reward or quest mark behind.
func (s *Server) tryWaterGathering(ctx context.Context, c *Session, click uint16, ev *assets.Event, branch int, now time.Time) (bool, error) {
	plan, involved := world.WaterGathering(c.character.Map, ev, branch)
	if !involved {
		return false, nil
	}
	es := &eventSession{mapID: c.character.Map, click: click, ev: ev, branch: branch}
	c.event = es
	if err := c.send([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1}); err != nil {
		return true, err
	}
	reject := func() (bool, error) {
		if err := c.send(headBanner("Cannot gather now. Check bag space and wait for the resource to recover.")); err != nil {
			return true, err
		}
		return true, s.finishEvent(c, es)
	}
	limit, known := s.stackLimit(plan.Item)
	if plan.Timer == 0 || !known {
		return reject()
	}
	next, adds, err := s.Store.GatherWater(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, plan.Timer, plan.Item, limit, now, func(stored *game.Character) bool {
		return stored.Map == es.mapID && s.World.FindBranchAt(stored, c.view, es.mapID, ev, world.TriggerEntry, 0, 0, -1, now) == branch
	}, c.character.Clone())
	if errors.Is(err, store.ErrGatheringUnavailable) || errors.Is(err, game.ErrInventoryFull) {
		return reject()
	}
	if err != nil {
		return true, errors.Join(err, s.cancelInteraction(c))
	}
	s.adoptSavedCharacter(c, next)
	packets := [][]byte{next.Bag.AdditionPacket(adds)}
	packets = append(packets, s.World.QuestUpdate(c.view, uint32(plan.Timer), next.Quests[uint32(plan.Timer)])...)
	if err := s.sendAll(c, packets); err != nil {
		return true, err
	}
	return true, s.finishEvent(c, es)
}
