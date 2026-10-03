package server

import (
	"context"
	"sort"
	"strings"

	"wonderland-go/internal/game"
	"wonderland-go/internal/world"
)

// Authored destinations from GmManager.TownDirectory. These are the legacy
// command aliases and coordinates, not a table inferred from scene names.
var townDestinations = map[string]world.Destination{
	"welling":   {Map: 10001, X: 800, Y: 750},
	"kelan":     {Map: 10011, X: 1000, Y: 1000},
	"holy":      {Map: 10016, X: 1200, Y: 950},
	"kyoto":     {Map: 10041, X: 1300, Y: 1100},
	"changan":   {Map: 10051, X: 1400, Y: 1200},
	"chang_an":  {Map: 10051, X: 1400, Y: 1200},
	"rome":      {Map: 10061, X: 1100, Y: 1000},
	"maya":      {Map: 10071, X: 900, Y: 900},
	"inca":      {Map: 10081, X: 800, Y: 850},
	"bangkok":   {Map: 10091, X: 1200, Y: 1100},
	"southpole": {Map: 10021, X: 1000, Y: 1000},
	"ghostisle": {Map: 10026, X: 800, Y: 800},
	"carnie":    carnie,
	"pirate":    {Map: game.MapID10036, X: 1038, Y: 2235},
	"kaohsiung": {Map: 10003, X: 1000, Y: 1000},
	"jail":      {Map: game.MapID10000, X: 600, Y: 600},
}

// Chat remains available during gameplay interactions. Travel must not let a
// command bypass their owner or move a character before map acknowledgment.
func commandTravelAvailable(c *Session) bool {
	return c.character != nil && c.ready && c.battle == nil && c.event == nil && !c.storm && c.beach == nil
}

// visitCarnie is shared by AC5:17 and the public chat command. Keep the original
// return point when already visiting; don't change it on a failed durable warp.
func (s *Server) visitCarnie(ctx context.Context, c *Session) error {
	if !commandTravelAvailable(c) {
		return nil
	}
	if _, ok := s.World.Map(carnie.Map); !ok {
		return nil
	}
	origin := world.Destination{Map: c.character.Map, X: c.character.X, Y: c.character.Y}
	err := s.commandTeleport(ctx, c, carnie)
	if origin.Map != carnie.Map && c.character.Map == carnie.Map {
		c.carnieReturn = &origin
	}
	return err
}

// publicDismount ports RideVehicle("") and AC02's shore relocation. It does
// not consume a raft, reset its wear, or rest a mounted companion pet.
func (s *Server) publicDismount(ctx context.Context, c *Session) error {
	if err := s.dismountVehicle(ctx, c); err != nil {
		return err
	}
	if c.character.Map == game.MapID10036 {
		return s.commandTeleport(ctx, c, townDestinations["pirate"])
	}
	return nil
}

func (s *Server) gmTown(ctx context.Context, c *Session, words []string) error {
	if len(words) < 2 {
		return nil
	}
	if dst, ok := townDestinations[strings.ToLower(words[1])]; ok {
		return s.commandTeleport(ctx, c, dst)
	}
	return nil
}

// SummonAllPlayers moves a stable snapshot: each teleport removes its target
// from the published world. Each character commits independently as in C#;
// one failed target must not prevent later targets from being processed.
func (s *Server) summonAll(ctx context.Context, gm *Session) error {
	dst := world.Destination{Map: gm.character.Map, X: gm.character.X, Y: gm.character.Y}
	targets := make([]*Session, 0, len(s.world))
	for _, c := range s.world {
		if c != gm {
			targets = append(targets, c)
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].info.ID < targets[j].info.ID })
	for _, c := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !commandTravelAvailable(c) {
			continue
		}
		if err := s.commandTeleport(ctx, c, dst); err != nil {
			s.Log.Error("GM summon failed", "gm", gm.character.ID, "character", c.character.ID, "error", err)
			// Reconnect after a failed transition for a fresh durable snapshot.
			c.conn.Close()
		}
	}
	return nil
}
