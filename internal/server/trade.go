package server

import (
	"context"
	"errors"
	"math"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

const tradeRequestTTL = time.Minute

type tradeRequest struct {
	from *Session
	at   time.Time
}
type tradeSession struct {
	players  [2]*Session
	offers   [2]game.TradeOffer
	accepted [2]bool
}

func (t *tradeSession) side(c *Session) int {
	if t.players[0] == c {
		return 0
	}
	return 1
}
func tradeMessage(message string) []byte {
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(message)
	return p
}
func tradeAvailable(c *Session) bool {
	return c != nil && c.ready && c.character.Preferences().TradeAllowed && c.battle == nil && c.event == nil && !c.storm && c.beach == nil && c.trade == nil && c.stall == nil
}
func tradeNear(a, b *Session) bool {
	return sameScene(a, b) && math.Hypot(float64(int(a.character.X)-int(b.character.X)), float64(int(a.character.Y)-int(b.character.Y))) <= tradeRangePixels
}

// tradeCommand ports AC25; caller holds worldMu. The native handler replaces the
// whole offer on each item/gold update and AC25:5 has no server-side effect.
func (s *Server) tradeCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.TradeRequest:
		if len(p) != 6 {
			return protocol.ErrMalformed
		}
		return s.requestTrade(c, r.U32())
	case protocol.TradeReply:
		if len(p) != 3 {
			return protocol.ErrMalformed
		}
		if p[2] == protocol.TradeReplyAccepted {
			return s.acceptTrade(c)
		}
		s.cancelTrade(c)
		return nil
	case protocol.TradeItemOffer:
		if len(p) != 5 {
			return protocol.ErrMalformed
		}
		if c.trade == nil {
			return nil
		}
		slot, count := p[3], p[4] // Trade window slot p[2] is unused by the reference.
		if count == 0 {
			return s.offerTrade(c, game.TradeOffer{})
		}
		if slot < 1 || slot > game.BagSize {
			return nil
		}
		item := c.character.Bag[slot-1]
		if item.Empty() || item.Locked {
			return nil
		}
		if _, known := s.Assets.Items[item.ID]; !known {
			return nil
		}
		return s.offerTrade(c, game.TradeOffer{Items: []game.TradeItem{{Slot: slot, Count: min(count, item.Count), Item: item}}})
	case protocol.TradeGoldOffer:
		if len(p) != 6 {
			return protocol.ErrMalformed
		}
		return s.offerTrade(c, game.TradeOffer{Gold: min(r.U32(), c.character.Gold)})
	case protocol.TradeOfferNoOp:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return nil
	case protocol.TradeConfirm:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.confirmTrade(ctx, c)
	case protocol.TradeCancel:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		s.cancelTrade(c)
		return nil
	default:
		return ErrUnsupported
	}
}

func (s *Server) requestTrade(c *Session, id uint32) error {
	target := s.mapPlayer(c, id)
	if target == nil || target == c {
		return nil
	}
	if !tradeAvailable(c) || !tradeAvailable(target) {
		return c.send(tradeMessage("Either you or the target is already busy."))
	}
	if !tradeNear(c, target) {
		return c.send(tradeMessage("You must be near the target to trade."))
	}
	if req := target.tradeRequest; req != nil && time.Since(req.at) < tradeRequestTTL {
		// Do not replace a prompt with another requester: the reply contains no ID.
		if req.from == c {
			return nil
		}
		return c.send(tradeMessage("The target already has a pending trade request."))
	}
	target.tradeRequest = &tradeRequest{from: c, at: time.Now()}
	s.sendOrClose(target, protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(c.character.ID))
	return c.send(tradeMessage("Trade request sent to " + target.character.Name + "."))
}

func (s *Server) acceptTrade(c *Session) error {
	req := c.tradeRequest
	c.tradeRequest = nil
	if req == nil {
		return nil
	}
	if time.Since(req.at) > tradeRequestTTL {
		s.sendOrClose(req.from, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
		s.sendOrClose(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
		return nil
	}
	from := req.from
	if !tradeAvailable(c) || !tradeAvailable(from) || s.world[from.info.ID] != from || !tradeNear(c, from) {
		s.sendOrClose(from, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
		s.sendOrClose(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
		return c.send(tradeMessage("The trading partner is no longer available."))
	}
	s.clearTradeRequests(c)
	s.clearTradeRequests(from)
	run := &tradeSession{players: [2]*Session{from, c}}
	from.trade, c.trade = run, run
	s.sendOrClose(from, protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(c.character.ID))
	s.sendOrClose(c, protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(from.character.ID))
	return nil
}

func (s *Server) clearTradeRequests(c *Session) {
	if pending := c.tradeRequest; pending != nil {
		c.tradeRequest = nil
		s.sendOrClose(pending.from, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
		s.sendOrClose(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
	}
	for _, peer := range s.world {
		if peer.tradeRequest != nil && peer.tradeRequest.from == c {
			peer.tradeRequest = nil
			s.sendOrClose(peer, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
			s.sendOrClose(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
		}
	}
}

func (s *Server) cancelTrade(c *Session) {
	s.clearTradeRequests(c)
	run := c.trade
	if run == nil {
		return
	}
	for _, player := range run.players {
		if player.trade == run {
			player.trade = nil
		}
	}
	for _, player := range run.players {
		s.sendOrClose(player, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyRejected})
	}
}

func (s *Server) offerTrade(c *Session, offer game.TradeOffer) error {
	run := c.trade
	if run == nil {
		return nil
	}
	side := run.side(c)
	run.offers[side] = offer
	run.accepted = [2]bool{} // Any revision invalidates both confirmations.
	p := protocol.Builder{protocol.CommandTrade, protocol.TradeItemOffer}.U32(offer.Gold).U8(byte(len(offer.Items)))
	for _, it := range offer.Items {
		p = p.U16(it.Item.ID).U8(it.Count).U8(it.Item.Damage)
	}
	s.sendOrClose(run.players[1-side], p)
	return nil
}

func (s *Server) confirmTrade(ctx context.Context, c *Session) error {
	run := c.trade
	if run == nil {
		return nil
	}
	a, b := run.players[0], run.players[1]
	if !a.ready || !b.ready || a.battle != nil || b.battle != nil || !tradeNear(a, b) {
		s.cancelTrade(c)
		return nil
	}
	run.accepted[run.side(c)] = true
	if !run.accepted[0] || !run.accepted[1] {
		return nil
	}
	var result game.TradeResult
	err := s.Store.UpdateCharacterPair(ctx, store.CharacterRef{Account: a.account.ID, ID: a.character.ID}, store.CharacterRef{Account: b.account.ID, ID: b.character.ID}, func(first, second *game.Character) error {
		var err error
		if err = game.PreserveItemLocks(*a.character, first); err != nil {
			return err
		}
		if err = game.PreserveItemLocks(*b.character, second); err != nil {
			return err
		}
		result, err = game.Exchange(*first, *second, run.offers, s.Assets.Items)
		if err != nil {
			return err
		}
		*first, *second = result.Characters[0], result.Characters[1]
		return nil
	})
	if err != nil {
		s.cancelTrade(c)
		if errors.Is(err, game.ErrTradeChanged) || errors.Is(err, game.ErrTradeGoldLimit) || errors.Is(err, game.ErrInventoryFull) || errors.Is(err, game.ErrInvalidItem) || errors.Is(err, game.ErrItemLocked) {
			for _, player := range run.players {
				s.sendOrClose(player, tradeMessage("Trade failed: "+err.Error()+"."))
			}
			return nil
		}
		return err
	}
	// Adopt both results only after their shared transaction has committed.
	old := [2]game.Character{a.character.Clone(), b.character.Clone()}
	for i, player := range run.players {
		s.adoptSavedCharacter(player, result.Characters[i])
		player.trade = nil
	}
	for i, player := range run.players {
		var packets [][]byte
		if old[i].ActiveVehicle != 0 && player.character.ActiveVehicle == 0 {
			dismount := vehicleDismountPacket(&old[i])
			s.broadcastWorld(player, dismount)
			packets = append(packets, dismount)
		}
		for _, it := range run.offers[i].Items {
			packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, it.Slot, it.Count})
		}
		if len(result.Additions[i]) > 0 {
			packets = append(packets, player.character.Bag.AdditionPacket(result.Additions[i]))
		}
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(player.character.Gold), []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCompleted}, tradeMessage("Trade completed successfully."))
		s.sendOrClose(player, packets...)
	}
	return nil
}
