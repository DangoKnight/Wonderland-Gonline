package server

import (
	"slices"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func (s *Server) mapTransitionReward(c *game.Character, to uint16) (*assets.MapTransition, []game.Addition, error) {
	for _, rule := range s.Assets.MapTransitions {
		if c.Map != rule.FromMap || to != rule.Destination.Map || rule.Quest == 0 || len(rule.Items) == 0 || c.Quests[rule.Quest].State == game.Completed {
			continue
		}
		for _, item := range c.Bag {
			if !item.Empty() && slices.Contains(rule.SkipIfOwned, item.ID) {
				return nil, nil, nil
			}
		}
		for _, grant := range rule.Items {
			if _, known := s.Assets.Items[grant.ItemID]; !known {
				return nil, nil, nil
			}
		}
		var adds []game.Addition
		for _, grant := range rule.Items {
			definition, known := s.Assets.Items[grant.ItemID]
			if !known {
				return nil, nil, nil
			}
			added, err := c.Bag.Grant(game.Item{ID: grant.ItemID}, int(grant.Quantity), definition.StackLimit(), s.Assets.Items)
			if err != nil {
				return nil, nil, err
			}
			adds = append(adds, added...)
		}
		now := time.Now().UTC()
		if c.Quests == nil {
			c.Quests = map[uint32]game.Quest{}
		}
		c.Quests[rule.Quest] = game.Quest{ID: rule.Quest, State: game.Completed, Step: 1, StartedAt: now, CompletedAt: &now}
		return &rule, adds, nil
	}
	return nil, nil, nil
}
