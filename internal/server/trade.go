package server

import (
	"context"
	"errors"
	"math"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// tradeOfferLimit is the trade form's 5 × 4 offer grid (FUN_003658a8).
const tradeOfferLimit = 20

// tradeSession is one open trade. Each side locks its offer once (AC25:2)
// and then accepts (AC25:3 1); the exchange runs when both have accepted.
type tradeSession struct {
	players  [2]*Session
	offers   [2]game.TradeOffer
	offered  [2]bool
	accepted [2]bool
}

func (t *tradeSession) side(c *Session) int {
	if t.players[0] == c {
		return 0
	}
	return 1
}

// tradeMessage is a notice shared by the trading interactions (stalls,
// manufacturing, social commands).
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
func tradeEnded(result byte) []byte {
	return []byte{protocol.CommandTrade, protocol.TradeEnded, result}
}

// tradeCommand ports AC25 as aLogin speaks it; caller holds worldMu.
func (s *Server) tradeCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.TradeRequest:
		if len(p) != 6 {
			return protocol.ErrMalformed
		}
		return s.requestTrade(c, protocol.NewReader(p[2:]).U32())
	case protocol.TradeOffer:
		if len(p) < 6 || len(p)-6 > tradeOfferLimit {
			return protocol.ErrMalformed
		}
		return s.offerTrade(c, protocol.NewReader(p[2:]).U32(), p[6:])
	case protocol.TradeDecision:
		if len(p) != 3 {
			return protocol.ErrMalformed
		}
		switch p[2] {
		case protocol.TradeDecisionAccept:
			return s.acceptTrade(ctx, c)
		case protocol.TradeDecisionCancel:
			// The canceller's window is already closed (FUN_00367dfc).
			s.endTrade(c, 0, false, protocol.TradeResultPartnerCanceled)
			return nil
		}
		return protocol.ErrMalformed
	default:
		return ErrUnsupported
	}
}

// requestTrade opens both windows at once: aLogin has no question for a
// trade (25/1 received opens the form, FUN_003c6b18 → FUN_00367d80); the
// target's trade setting is the consent.
func (s *Server) requestTrade(c *Session, id uint32) error {
	if c.trade != nil {
		return c.send(tradeEnded(protocol.TradeResultAlreadyTrading))
	}
	target := s.mapPlayer(c, id)
	switch {
	case target == c:
		return nil
	case target == nil:
		for _, peer := range s.world {
			if peer.character.ID == id {
				return c.send(tradeEnded(protocol.TradeResultDifferentMaps))
			}
		}
		return c.send(tradeEnded(protocol.TradeResultDisconnected))
	case !tradeAvailable(c) || !tradeAvailable(target):
		return c.send(tradeEnded(protocol.TradeResultBusy))
	case !tradeNear(c, target):
		return c.send(tradeEnded(protocol.TradeResultUnavailable))
	}
	run := &tradeSession{players: [2]*Session{c, target}}
	c.trade, target.trade = run, run
	s.sendOrClose(target, protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(c.character.ID))
	return c.send(protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(target.character.ID))
}

// offerTrade is the Confirm button (FUN_003680f4): the gold and the bag
// slots of whole stacks, locked until the trade ends.
func (s *Server) offerTrade(c *Session, gold uint32, slots []byte) error {
	run := c.trade
	if run == nil {
		return nil
	}
	side := run.side(c)
	if run.offered[side] {
		return nil
	}
	seen := map[byte]bool{}
	for _, slot := range slots {
		if slot < 1 || slot > game.BagSize || seen[slot] {
			return protocol.ErrMalformed
		}
		seen[slot] = true
	}
	offer := game.TradeOffer{Gold: gold}
	for _, slot := range slots {
		item := c.character.Bag[slot-1]
		if _, known := s.Assets.Items[item.ID]; item.Empty() || item.Locked || !known {
			s.endTrade(c, protocol.TradeResultUnavailable, true, protocol.TradeResultPartnerCanceled)
			return nil
		}
		offer.Items = append(offer.Items, game.TradeItem{Slot: slot, Count: item.Count, Item: item})
	}
	if gold > c.character.Gold {
		s.endTrade(c, protocol.TradeResultNotEnoughGold, true, protocol.TradeResultPartnerCanceled)
		return nil
	}
	run.offers[side], run.offered[side] = offer, true
	s.sendOrClose(run.players[1-side], tradeOfferPacket(offer))
	return nil
}

// tradeOfferPacket is AC25:3 as FUN_003c704c reads it: the gold, then per
// item its ID, count and damage and the bag record's 26 metadata bytes with
// the u32/u16 pair (bytes 2..7) ahead of bytes 0..1.
func tradeOfferPacket(offer game.TradeOffer) []byte {
	p := protocol.Builder{protocol.CommandTrade, protocol.TradePartnerOffer}.U32(offer.Gold)
	for _, it := range offer.Items {
		m := it.Item.Metadata
		p = p.U16(it.Item.ID).U8(it.Count).U8(it.Item.Damage).Bytes(m[2:8]).Bytes(m[0:2]).Bytes(m[8:])
	}
	return p
}

// acceptTrade is the Trade button (0x368000): shown once both offers are
// locked; the exchange commits when both sides have pressed it.
func (s *Server) acceptTrade(ctx context.Context, c *Session) error {
	run := c.trade
	if run == nil || !run.offered[0] || !run.offered[1] {
		return nil
	}
	a, b := run.players[0], run.players[1]
	if !a.ready || !b.ready || a.battle != nil || b.battle != nil || !tradeNear(a, b) {
		s.endTrade(c, protocol.TradeResultUnavailable, true, protocol.TradeResultUnavailable)
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
		code := byte(protocol.TradeResultUnavailable)
		if errors.Is(err, game.ErrInventoryFull) {
			code = protocol.TradeResultInventoryFull
		}
		s.endTrade(c, code, true, code)
		if errors.Is(err, game.ErrTradeChanged) || errors.Is(err, game.ErrTradeGoldLimit) || errors.Is(err, game.ErrInventoryFull) || errors.Is(err, game.ErrInvalidItem) || errors.Is(err, game.ErrItemLocked) {
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
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(player.character.Gold), tradeEnded(protocol.TradeResultCompleted))
		s.sendOrClose(player, packets...)
	}
	return nil
}

// cancelTrade ends c's trade from the server's side: both windows close.
func (s *Server) cancelTrade(c *Session) {
	s.endTrade(c, protocol.TradeResultCanceled, true, protocol.TradeResultPartnerCanceled)
}

// endTrade closes the trade, telling c (when its window is still open) and
// its partner why.
func (s *Server) endTrade(c *Session, own byte, tellOwn bool, partner byte) {
	run := c.trade
	if run == nil {
		return
	}
	for _, player := range run.players {
		if player.trade == run {
			player.trade = nil
		}
	}
	other := run.players[1-run.side(c)]
	if tellOwn {
		s.sendOrClose(c, tradeEnded(own))
	}
	s.sendOrClose(other, tradeEnded(partner))
}
