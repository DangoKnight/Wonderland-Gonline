package server

import (
	"context"
	"errors"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

const (
	propIntactFrame           = 0
	propBrokenFrame           = 1
	scriptedPropResetInterval = time.Minute
)

func propFrame(click uint16, state byte) []byte {
	return protocol.Builder{protocol.CommandScene, protocol.SceneActorState}.U16(click).U8(state)
}
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
	if !known {
		return true, s.sendAll(c, [][]byte{headBanner("The node reward is unavailable."), {protocol.CommandEvent, protocol.EventResume}})
	}
	next, adds, err := s.Store.ClaimMapProp(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, c.character.Map, n.ClickID, game.Item{ID: reward.Item, Count: reward.Count}, def.StackLimit(), time.Now(), time.Duration(pool.RespawnSeconds)*time.Second, s.Assets.Items, *c.character)
	if errors.Is(err, store.ErrPropEmpty) || errors.Is(err, game.ErrInventoryFull) {
		return true, s.sendAll(c, [][]byte{headBanner(err.Error()), {protocol.CommandEvent, protocol.EventResume}})
	}
	if err != nil {
		return true, err
	}
	s.adoptSavedCharacter(c, next)
	s.closeStall(c)
	s.cancelTrade(c)
	s.publishProp(next.Map, n.ClickID, propBrokenFrame)
	return true, s.sendAll(c, [][]byte{next.Bag.AdditionPacket(adds), {protocol.CommandEvent, protocol.EventStepComplete}, {protocol.CommandEvent, protocol.EventResume}})
}
func (s *Server) publishProp(mapID, click uint16, frame byte) {
	for _, c := range s.world {
		if !c.ready || c.character == nil || c.tentOwner != 0 || c.character.Map != mapID || c.view == nil || c.view.Hidden[click] || !s.World.VisibleIn(c.character, c.view, mapID, click) {
			continue
		}
		c.view.Props[click] = int32(frame)
		s.sendOrClose(c, propFrame(click, frame))
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
		if err = c.send(propFrame(p.ClickID, p.Frame)); err != nil {
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
