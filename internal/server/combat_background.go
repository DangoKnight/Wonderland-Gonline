package server

import (
	"math"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// Unknown maps retain the historical wire selector and native client fallback.
const compatibilityBattleBackground = 1

func (s *Server) acceptClientBackdrops(c *Session, p []byte) error {
	if !protocol.IsBattleBackdropCapability(p, protocol.HandshakeBackdropAccept) {
		return protocol.ErrMalformed
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if c.connectionOffered != 0 || c.connectionNegotiated || !c.backdropOffered || c.battle != nil {
		return protocol.ErrMalformed
	}
	if err := c.send(protocol.BattleBackdropCapability(protocol.HandshakeBackdropEnabled)); err != nil {
		return err
	}
	c.clientBackdrops = true
	return nil
}

func battleBackdropLocation(c *Session, run *battleRun) protocol.BattleBackdropLocation {
	l := protocol.BattleBackdropLocation{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
	if run != nil && run.event != nil {
		l.Map = run.event.mapID
	}
	return l
}

// Resolve once only when a recipient still needs a native server selector.
func (s *Server) legacyBattleBackground(c *Session, run *battleRun) byte {
	for _, m := range run.members {
		if !m.c.clientBackdrops {
			return s.battleBackground(c, run)
		}
	}
	return compatibilityBattleBackground
}

func (s *Server) battleBackground(c *Session, run *battleRun) byte {
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
			return byte(policy.Value)
		}
	}
	if s.Assets != nil {
		if backdrop, ok := s.Assets.BattleBackdrops[ctx.Map]; ok {
			return backdrop.At(c.character.X, c.character.Y)
		}
	}
	if c.character.Map <= math.MaxUint8 {
		return byte(c.character.Map)
	}
	return compatibilityBattleBackground
}
