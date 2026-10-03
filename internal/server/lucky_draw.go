package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

// FUN_00175cf0 consumes used count, reward count, then UInt16 ID/byte quantity
// rows. The client derives remaining draws as three minus used, not a balance.
func (s *Server) luckyDrawCatalogPacket(c *game.Character, now time.Time) []byte {
	pool := s.Assets.LuckyDraw
	used := game.LuckyDrawsPerDay - c.LuckyDraw.Remaining(now)
	p := protocol.Builder{protocol.CommandLuckyDraw, protocol.LuckyDrawMode, protocol.LuckyDrawCatalog, used, byte(len(pool.Rewards))}
	for _, reward := range pool.Rewards {
		p = p.U16(reward.ID).U8(byte(reward.Quantity))
	}
	return p
}

func (s *Server) luckyDrawCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.LuckyDrawRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.LuckyDrawSpin {
		return ErrUnsupported
	}
	s.Log.Debug("Lucky Draw spin requested", "session", c.info.ID, "character", c.character.ID, "map", c.character.Map, "event_active", c.event != nil, "storm", c.storm, "beach", c.beach != nil)
	return s.drawLucky(ctx, c, time.Now())
}

// Caller holds worldMu. Store rechecks durable allowance and inventory capacity.
func (s *Server) drawLucky(ctx context.Context, c *Session, now time.Time) error {
	if c.event != nil || c.storm || c.beach != nil {
		s.Log.Debug("Lucky Draw ignored", "session", c.info.ID, "reason", "active interaction")
		return nil
	}
	pool := s.Assets.LuckyDraw
	if pool.TotalWeight <= 0 {
		s.Log.Debug("Lucky Draw unavailable", "session", c.info.ID, "reason", "empty reward pool")
		return c.send(headBanner("Lucky Draw rewards are unavailable."))
	}
	roll, err := rand.Int(rand.Reader, big.NewInt(pool.TotalWeight))
	if err != nil {
		return err
	}
	reward, ok := pool.RewardForRoll(roll.Int64())
	if !ok {
		return errors.New("invalid Lucky Draw pool")
	}
	limit, known := s.stackLimit(reward.ID)
	if !known {
		return errors.New("unknown Lucky Draw reward")
	}
	next, adds, err := s.Store.DrawLucky(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, reward.ID, reward.Quantity, limit, now)
	if errors.Is(err, store.ErrLuckyDrawLimit) {
		s.Log.Debug("Lucky Draw refused", "session", c.info.ID, "reason", "daily limit")
		c.character.LuckyDraw = next.LuckyDraw
		return s.sendAll(c, [][]byte{headBanner("You have used all three Lucky Draws for today."), s.luckyDrawCatalogPacket(c.character, now)})
	}
	if errors.Is(err, game.ErrInventoryFull) {
		s.Log.Debug("Lucky Draw refused", "session", c.info.ID, "reason", "inventory full")
		return c.send(headBanner("Please free an inventory slot. Lucky Draw retained."))
	}
	if err != nil {
		return err
	}
	*c.character = next
	s.Log.Debug("Lucky Draw committed", "session", c.info.ID, "character", next.ID, "reward", reward.ID, "quantity", reward.Quantity, "result_slot", reward.Slot, "remaining", next.LuckyDraw.Remaining(now))
	// Persist first. Failed receipts cannot restore a consumed draw or duplicate its reward.
	return s.sendAll(c, [][]byte{
		next.Bag.AdditionPacket(adds),
		{protocol.CommandLuckyDraw, protocol.LuckyDrawMode, protocol.LuckyDrawResult, reward.Slot, game.LuckyDrawsPerDay - next.LuckyDraw.Remaining(now)},
		headBanner(fmt.Sprintf("Lucky Draw: received [%s] x%d.", s.itemName(reward.ID), reward.Quantity)),
	})
}

// Refresh remaining allowance for ready characters at UTC midnight. No state is
// saved until a draw succeeds; reset eligibility is derived from the saved day.
func (s *Server) refreshLuckyDrawCatalog(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if s.Assets.LuckyDraw.TotalWeight <= 0 {
		return
	}
	for _, c := range s.world {
		if !c.ready {
			continue
		}
		if err := c.send(s.luckyDrawCatalogPacket(c.character, now)); err != nil {
			c.conn.Close()
		}
	}
}

func (s *Server) runLuckyDrawResets(ctx context.Context) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case now := <-timer.C:
			s.refreshLuckyDrawCatalog(now)
		}
	}
}
