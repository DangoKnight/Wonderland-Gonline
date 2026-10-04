package server

import (
	"math/rand/v2"
	"strings"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func (s *Server) chestPool(mapID, click uint16) *assets.ChestPool {
	var mapPool, fallback *assets.ChestPool
	name := ""
	if m, ok := s.World.Map(mapID); ok {
		if n, ok := m.NPC(click); ok {
			name = strings.ToLower(n.Name)
		}
	}
	for i := range s.Assets.ChestPools {
		pool := &s.Assets.ChestPools[i]
		if pool.MapID != 0 {
			if pool.MapID == mapID {
				mapPool = pool
			}
			continue
		}
		category := strings.ToLower(strings.TrimSpace(pool.Category))
		if category == "default_chest" {
			fallback = pool
			continue
		}
		names := []string{category}
		// Verified category aliases from ChestDropManager.RollDrop.
		switch category {
		case "medicine":
			names = []string{"cabinet", "shelf", "bick"}
		case "headband":
			names = []string{"headband", "bush"}
		case "ore":
			names = []string{"mine", "ore", "vein"}
		}
		for _, match := range names {
			if match != "" && strings.Contains(name, match) {
				return pool
			}
		}
	}
	if mapPool != nil {
		return mapPool
	}
	return fallback
}

func rollChestReward(pool assets.ChestPool) assets.ChestReward {
	var total uint32
	for _, r := range pool.Rewards {
		total += r.Weight
	}
	draw := uint32(rand.IntN(int(total)))
	for _, r := range pool.Rewards {
		if draw < r.Weight {
			return r
		}
		draw -= r.Weight
	}
	return pool.Rewards[len(pool.Rewards)-1]
}

// Keep loaded clients' props consistent with durable cooldowns. Expiry is a
// derived state: reconnects still read the saved timestamp without a reset write.
func (s *Server) respawnChests(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	for _, c := range s.friendSessions {
		if !gmIdle(c) || c.view == nil {
			continue
		}
		m, ok := s.Assets.Maps[c.character.Map]
		if !ok {
			continue
		}
		for _, ev := range m.Events {
			expiry, ok := c.character.ChestRespawns[world.ChestKey(c.character.Map, ev.ClickID)]
			if !ok || now.Before(expiry) || c.view.Props[ev.ClickID] != 1 {
				continue
			}
			c.view.Props[ev.ClickID] = 0
			s.sendOrClose(c, protocol.Builder{protocol.CommandScene, protocol.SceneActorState}.U16(ev.ClickID).U8(0))
		}
	}
}
