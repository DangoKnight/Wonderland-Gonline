package server

import (
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

// Unknown maps retain the historical wire selector and native client fallback.
const compatibilityBattleBackground = 1

func (s *Server) battleBackground(c *Session, run *battleRun) uint16 {
	ctx := world.StoryContext{Map: c.character.Map}
	if run != nil && run.event != nil {
		es := run.event
		ctx.Map, ctx.Actor = es.mapID, es.click
		if es.ev != nil {
			ctx.Event = es.ev.ClickID
			if es.branch >= 0 && es.branch < len(es.ev.Branches) {
				ctx.Branch = es.ev.Branches[es.branch].Index
			}
		}
	}
	if s.World != nil {
		if policy, ok := s.World.StoryFirst(assets.StoryBattleBackground, c.character, c.view, ctx); ok {
			return uint16(policy.Value)
		}
	}
	if c.character.Map < game.MapID10000 {
		return c.character.Map
	}
	return compatibilityBattleBackground
}
