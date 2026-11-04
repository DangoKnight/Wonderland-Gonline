package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
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
		return false, c.send(serverwire.HeadBanner(err.Error()))
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
		serverwire.HeadBanner(fmt.Sprintf("Obtain %d gold", gold)),
		{protocol.CommandEvent, protocol.EventStepComplete},
		serverwire.PropFrame(es.ev.ClickID, propBrokenFrame),
	})
}

func (s *Server) chestPool(mapID, click uint16) *assets.ChestPool {
	var mapPool, fallback, cratePool *assets.ChestPool
	name := ""
	kind := ""
	if m, ok := s.World.Map(mapID); ok {
		if n, ok := m.NPC(click); ok {
			name = strings.ToLower(n.Name)
			if n.Template <= 0xffff {
				kind = s.Assets.ChestPropCategories[uint16(n.Template)]
			}
		}
	}
	if kind == "" {
		for _, word := range []string{"crate", "cask", "barrel", "box", "cargo", "wreckage", "driftwood"} {
			if strings.Contains(name, word) {
				kind = "crate"
				break
			}
		}
		if kind == "" && strings.Contains(name, "ches") {
			kind = "chest"
		}
	}
	beach := mapID >= game.ShipwreckBeachFirstMapID && mapID <= game.ShipwreckBeachLastMapID
	for i := range s.Assets.ChestPools {
		pool := &s.Assets.ChestPools[i]
		if pool.MapID != 0 {
			if pool.MapID == mapID {
				mapPool = pool
			}
			continue
		}
		category := strings.ToLower(strings.TrimSpace(pool.Category))
		switch category {
		case "beach_crate":
			if kind == "crate" {
				if beach {
					return pool
				}
				cratePool = pool
			}
			continue
		case "beach_chest":
			if beach && kind == "chest" {
				return pool
			}
			continue
		case "default_crate":
			if cratePool == nil {
				cratePool = pool
			}
			continue
		}
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
	if kind == "crate" && cratePool != nil {
		return cratePool
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

const (
	propIntactFrame           = 0
	propBrokenFrame           = 1
	scriptedPropResetInterval = time.Minute
)

func (s *Server) harvestProp(ctx context.Context, c *Session, n world.NPC) (bool, error) {
	if c.tentOwner != 0 || !s.World.GatheringProp(c.character.Map, n) {
		return false, nil
	}
	pool := s.chestPool(c.character.Map, n.ClickID)
	if pool == nil || len(pool.Rewards) == 0 {
		return false, nil
	}
	reward := rollChestReward(*pool)
	def, known := s.Assets.Items[reward.Item]
	if !known && reward.Gold == 0 {
		return true, s.sendAll(c, [][]byte{serverwire.HeadBanner("The node reward is unavailable."), {protocol.CommandEvent, protocol.EventResume}})
	}
	item := game.Item{ID: reward.Item, Count: reward.Count}
	if reward.Gold > 0 {
		item = game.Item{}
	}
	next, _, err := s.Store.ClaimMapPropReward(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, c.character.Map, n.ClickID, item, def.StackLimit(), reward.Gold, time.Now(), time.Duration(pool.RespawnSeconds)*time.Second, s.Assets.Items, *c.character)
	if errors.Is(err, store.ErrPropEmpty) || errors.Is(err, store.ErrPropRewardUnavailable) || errors.Is(err, game.ErrInventoryFull) {
		return true, s.sendAll(c, [][]byte{serverwire.HeadBanner(err.Error()), {protocol.CommandEvent, protocol.EventResume}})
	}
	if err != nil {
		return true, err
	}
	s.adoptSavedCharacter(c, next)
	s.closeStall(c)
	s.cancelTrade(c)
	s.publishProp(next.Map, n.ClickID, propBrokenFrame)
	receipt := game.ActorAcquisitionPacket(item)
	if reward.Gold > 0 {
		receipt = protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold)
	}
	return true, s.sendAll(c, [][]byte{receipt, {protocol.CommandEvent, protocol.EventStepComplete}, {protocol.CommandEvent, protocol.EventResume}})
}
func (s *Server) publishProp(mapID, click uint16, frame byte) {
	for _, c := range s.world {
		if !c.ready || c.character == nil || c.tentOwner != 0 || c.character.Map != mapID || c.view == nil || c.view.Hidden[click] || !s.World.VisibleIn(c.character, c.view, mapID, click) {
			continue
		}
		c.view.Props[click] = int32(frame)
		s.sendOrClose(c, serverwire.PropFrame(click, frame))
	}
}
func (s *Server) syncMapProps(ctx context.Context, c *Session, now time.Time) error {
	if c.tentOwner != 0 {
		return nil
	}
	props, err := s.Store.ActiveMapProps(ctx, c.character.Map, now)
	if err != nil {
		return err
	}
	for _, p := range props {
		if _, ok := s.World.NPC(p.MapID, p.ClickID); !ok || c.view.Hidden[p.ClickID] || !s.World.VisibleIn(c.character, c.view, p.MapID, p.ClickID) {
			continue
		}
		c.view.Props[p.ClickID] = int32(p.Frame)
		if err = c.send(serverwire.PropFrame(p.ClickID, p.Frame)); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) respawnMapProps(ctx context.Context, now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	props, err := s.Store.ExpireMapProps(ctx, now)
	if err != nil {
		s.Log.Error("map prop respawn failed", "error", err)
		return
	}
	for _, p := range props {
		s.publishProp(p.MapID, p.ClickID, propIntactFrame)
	}
}
