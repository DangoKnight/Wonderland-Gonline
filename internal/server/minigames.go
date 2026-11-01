package server

import (
	"context"
	"math"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// startMinigame ports EVE opcode 9. The native outcome condition uses the
// game type as source and 0/1 as result; EVE owns rewards and unlocking.
func (s *Server) startMinigame(ctx context.Context, c *Session, es *eventSession, op world.Op) error {
	c.arcade = nil
	if op.D1 <= math.MaxUint8 && assets.ArcadeKindSupported(byte(op.D1)) {
		c.arcade = &arcadeSession{kind: byte(op.D1), mapID: c.character.Map, event: es}
	}
	seed := uint32(defaultMinigameSeed)
	if op.D2 > 0 {
		seed = uint32(op.D2) | minigameSeedHighByte
	}
	es.onMinigame = func(result byte) error {
		if c.event != es || c.character.Map != es.mapID {
			return nil
		}
		branch := s.World.FindBranch(c.character, c.view, es.mapID, es.ev, world.ConditionMinigameResult, op.D1, uint16(result), -1)
		if branch < 0 {
			return s.finishEvent(c, es)
		}
		return s.startEvent(ctx, c, es.click, es.ev, branch, true)
	}
	start := []byte{protocol.CommandMinigame, protocol.MinigameStart, byte(op.D1), byte(seed), byte(seed >> 8), byte(seed >> 16)}
	if op.D1 == protocol.MinigameThrow {
		start = append(start, throwSceneVariants[es.mapID])
	}
	return s.sendAll(c, [][]byte{start, {protocol.CommandEvent, protocol.EventHold}})
}

// throwSceneVariants pick the dodgeball game's scene. FUN_001ae968 draws
// battle scene 59000 + the start packet's sixth byte, which the EVE launch
// operations do not carry and no capture records. These compatibility
// choices give the two launches a fitting battle scene: the Mayan Tribe
// plays on Jungle (59090), Large Island on Beach (59050). Other maps send
// 0, scene 59000, which has no record: the original then draws no scene.
var throwSceneVariants = map[uint16]byte{
	12150: 90,
	60008: 50,
}

// minigameCommand handles native AC57:1. Reports have no game identifier; only
// the current event can accept one. Consume before invoking because an outcome
// may launch another minigame. Caller holds worldMu.
func (s *Server) minigameCommand(c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.MinigameResult {
		return ErrUnsupported
	}
	if len(p) != 3 || p[2] > 1 {
		return nil
	}
	es := c.event
	if c.arcade != nil && c.arcade.event == nil {
		c.arcade = nil
		return c.send([]byte{protocol.CommandMinigame, protocol.MinigameEnd})
	}
	if es == nil || es.onMinigame == nil || c.character.Map != es.mapID {
		return nil
	}
	c.arcade = nil
	outcome := es.onMinigame
	es.onMinigame = nil
	c.resultAck = true
	if err := c.send([]byte{protocol.CommandMinigame, protocol.MinigameEnd}); err != nil {
		return err
	}
	return outcome(p[2])
}
