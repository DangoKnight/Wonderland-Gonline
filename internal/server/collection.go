package server

import (
	"context"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func (s *Server) collectionTimers(mapID uint16, ev *assets.Event) []uint16 {
	seen := map[uint16]bool{}
	var timers []uint16
	for i := range ev.Branches {
		plan, _ := s.World.PlanEventGathering(mapID, ev, i)
		if plan.Timer != 0 && !seen[plan.Timer] {
			seen[plan.Timer] = true
			timers = append(timers, plan.Timer)
		}
	}
	return timers
}

// Native timer IDs can be reused by unrelated actors. Project only this spot's
// shared clock into a temporary condition view; owned timer history is ignored.
func collectionCharacter(c *game.Character, timers []uint16, expiry time.Time) *game.Character {
	if len(timers) == 0 {
		return c
	}
	next := c.Clone()
	if next.EventTimers == nil {
		next.EventTimers = map[uint16]time.Time{}
	}
	for _, id := range timers {
		if expiry.IsZero() {
			delete(next.EventTimers, id)
		} else {
			next.EventTimers[id] = expiry
		}
	}
	return &next
}

func (s *Server) collectionNode(mapID, click uint16, ev *assets.Event, nodes []store.ResourceKey) store.ResourceKey {
	if len(nodes) > 0 {
		return nodes[0]
	}
	if click != 0 {
		return store.ResourceKey{Kind: store.ResourceCollectionActor, MapID: mapID, NodeID: click}
	}
	return store.ResourceKey{Kind: store.ResourceCollectionArea, MapID: mapID, NodeID: ev.ClickID}
}

func (s *Server) findCollectionBranch(ctx context.Context, c *Session, node store.ResourceKey, ev *assets.Event, now time.Time) (int, error) {
	timers := s.collectionTimers(c.character.Map, ev)
	character := c.character
	if len(timers) > 0 {
		expiry, err := s.Store.ResourceExpiry(ctx, node)
		if err != nil {
			return -1, err
		}
		character = collectionCharacter(character, timers, expiry)
	}
	return s.World.FindBranchAt(character, c.view, c.character.Map, ev, world.TriggerEntry, 0, 0, -1, now), nil
}
