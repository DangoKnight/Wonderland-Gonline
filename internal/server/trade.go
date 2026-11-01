package server

import (
	"context"
	"errors"
	"fmt"
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
	// sent records the offer last pushed to each side. The native client appends
	// AC25:3 rows to the grid instead of replacing them, so an identical resend
	// duplicates items on screen.
	sent [2]game.TradeOffer
	// sentValid marks offers that have been pushed at least once.
	sentValid [2]bool
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
		return nil
	}
	s.tradeLog("rx", c, p)
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.TradeRequest:
		if len(p) != 6 {
			s.tradeWarn(c, p, "unsupported AC25:1 length")
			return nil
		}
		return s.requestTrade(c, r.U32())
	case protocol.TradeReply:
		// Native AC25:2 carries the offered gold followed by one byte per
		// offered inventory slot: 19 02 <gold u32> [<slot u8> ...]. The client
		// sends the whole offer in a single packet, so several slots are normal.
		if len(p) >= 6 {
			return s.offerTradeSlots(c, p[2:6], p[6:])
		}
		if len(p) == 3 {
			if p[2] == protocol.TradeReplyAccepted {
				return s.acceptTrade(c)
			}
			if p[2] == protocol.TradeReplyRejected {
				s.cancelTrade(c)
			}
			return nil
		}
		s.tradeWarn(c, p, "unsupported AC25:2 length")
		return nil
	case protocol.TradeItemOffer:
		if len(p) == 3 {
			// AC25:3 carries a single flag: 1 = confirm, 2 = cancel. The same
			// shape accepts an invitation while no session is open yet.
			switch p[2] {
			case protocol.TradeReplyAccepted:
				if c.trade == nil {
					return s.acceptTrade(c)
				}
				return s.confirmTrade(ctx, c)
			case protocol.TradeReplyRejected:
				s.cancelTrade(c)
			}
			return nil
		}
		if len(p) != 5 {
			s.tradeWarn(c, p, "unsupported AC25:3 length")
			return nil
		}
		if c.trade == nil {
			return nil
		}
		slot, count := p[3], p[4]
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
		if len(p) == 6 {
			return s.offerTrade(c, game.TradeOffer{Gold: min(r.U32(), c.character.Gold)})
		}
		if len(p) == 5 && c.trade != nil {
			slot, count := p[3], p[4]
			if slot < 1 || slot > game.BagSize || count == 0 {
				return nil
			}
			item := c.character.Bag[slot-1]
			if item.Empty() || item.Locked {
				return nil
			}
			return s.offerTrade(c, game.TradeOffer{Items: []game.TradeItem{{Slot: slot, Count: min(count, item.Count), Item: item}}})
		}
		s.tradeWarn(c, p, "unsupported AC25:4 length")
		return nil
	case protocol.TradeOfferNoOp:
		if len(p) != 2 {
			s.tradeWarn(c, p, "unsupported AC25:5 length")
		}
		return nil
	case protocol.TradeConfirm:
		if len(p) != 2 {
			s.tradeWarn(c, p, "unsupported AC25:6 length")
			return nil
		}
		return s.confirmTrade(ctx, c)
	case protocol.TradeCancel:
		if len(p) != 2 {
			s.tradeWarn(c, p, "unsupported AC25:7 length")
			return nil
		}
		s.cancelTrade(c)
		return nil
	case 10, 40:
		s.tradeSend(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
		return c.send(tradeMessage("Pet and equipment trading are not available yet."))
	}
	return nil
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
	run := &tradeSession{players: [2]*Session{c, target}}
	c.trade, target.trade = run, run
	c.tradeRequest, target.tradeRequest = nil, nil
	s.tradeSend(c, protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(target.character.ID))
	s.tradeSend(target, protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(c.character.ID))
	s.tradeSend(c, tradeWindow(c, target))
	s.tradeSend(target, tradeWindow(target, c))
	s.sendTradeOffers(run)
	return c.send(tradeMessage("Trade request sent to " + target.character.Name + "."))
}

func (s *Server) acceptTrade(c *Session) error {
	if c.trade != nil {
		return nil
	}
	req := c.tradeRequest
	c.tradeRequest = nil
	if req == nil {
		return nil
	}
	if time.Since(req.at) > tradeRequestTTL {
		s.tradeSend(req.from, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
		s.tradeSend(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
		return nil
	}
	from := req.from
	if !tradeAvailable(c) || !tradeAvailable(from) || s.world[from.info.ID] != from || !tradeNear(c, from) {
		s.tradeSend(from, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
		s.tradeSend(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
		return c.send(tradeMessage("The trading partner is no longer available."))
	}
	s.clearTradeRequests(c)
	s.clearTradeRequests(from)
	run := &tradeSession{players: [2]*Session{from, c}}
	from.trade, c.trade = run, run
	s.tradeSend(from, tradeWindow(from, c))
	s.tradeSend(c, tradeWindow(c, from))
	s.sendTradeOffers(run)
	return nil
}

func (s *Server) clearTradeRequests(c *Session) {
	if pending := c.tradeRequest; pending != nil {
		c.tradeRequest = nil
		s.tradeSend(pending.from, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
		s.tradeSend(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
	}
	for _, peer := range s.world {
		if peer.tradeRequest != nil && peer.tradeRequest.from == c {
			peer.tradeRequest = nil
			s.tradeSend(peer, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
			s.tradeSend(c, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
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
		s.tradeSend(player, []byte{protocol.CommandTrade, protocol.TradeReply, protocol.TradeReplyCancelled})
	}
}

// offerTradeSlots replaces the caller's offer from a native AC25:2 payload:
// offered gold plus one inventory slot per item. Slots that are out of range,
// empty or locked are dropped instead of failing the whole packet, because a
// rejected trade packet used to close the session.
func (s *Server) offerTradeSlots(c *Session, gold []byte, slots []byte) error {
	if c.trade == nil {
		return nil
	}
	amount := uint32(gold[0]) | uint32(gold[1])<<8 | uint32(gold[2])<<16 | uint32(gold[3])<<24
	offer := game.TradeOffer{Gold: min(amount, c.character.Gold)}
	for _, slot := range slots {
		if slot < 1 || int(slot) > game.BagSize {
			continue
		}
		item := c.character.Bag[slot-1]
		if item.Empty() || item.Locked {
			continue
		}
		if _, known := s.Assets.Items[item.ID]; !known {
			continue
		}
		offer.Items = append(offer.Items, game.TradeItem{Slot: slot, Count: item.Count, Item: item})
		if len(offer.Items) == game.BagSize {
			break
		}
	}
	return s.offerTrade(c, offer)
}

// tradeWarn reports a trade packet whose shape is not ported yet. The session
// keeps running: the native client retries or cancels on its own.
func (s *Server) tradeWarn(c *Session, p []byte, reason string) {
	if s.Log == nil {
		return
	}
	s.Log.Warn("trade packet ignored", "session", c.info.ID, "sub", p[1], "len", len(p), "reason", reason, "hex", fmt.Sprintf("% x", p))
}

func (s *Server) offerTrade(c *Session, offer game.TradeOffer) error {
	run := c.trade
	if run == nil {
		return nil
	}
	side := run.side(c)
	// The client re-sends its offer together with the confirmation. Treating
	// that echo as a revision would drop the partner's confirmation and force
	// a second click from both sides.
	if sameTradeOffer(run.offers[side], offer) {
		return nil
	}
	run.offers[side] = offer
	run.accepted = [2]bool{} // Any revision invalidates both confirmations.
	for _, player := range run.players {
		s.tradeSend(player, tradeWindow(player, run.players[1-run.side(player)]))
	}
	s.sendTradeOffers(run)
	return nil
}

// sameTradeOffer reports whether the client resent the offer it already had.
func sameTradeOffer(a, b game.TradeOffer) bool {
	if a.Gold != b.Gold || len(a.Items) != len(b.Items) {
		return false
	}
	for i := range a.Items {
		if a.Items[i].Slot != b.Items[i].Slot || a.Items[i].Count != b.Items[i].Count || a.Items[i].Item.ID != b.Items[i].Item.ID {
			return false
		}
	}
	return true
}

// sendTradeOffers tells each player what the other side currently offers.
// The client renders AC25:3 only: gold followed by one 30-byte record per item,
// where the record starts with the item id and keeps the quantity in byte 11.
// The trailing five zero bytes keep the client's record count ((len-5)/29) exact.
func (s *Server) sendTradeOffers(run *tradeSession) {
	for _, player := range run.players {
		side := run.side(player)
		partnerSide := 1 - side
		offer := run.offers[partnerSide]
		if run.sentValid[side] && sameTradeOffer(run.sent[side], offer) {
			continue // the client would append a duplicate row
		}
		run.sent[side] = offer
		run.sentValid[side] = true
		s.tradeSend(player, tradeOfferPacket(offer))
	}
}

// tradeOfferRecordSize is the native AC25:3 record width: id, damage, a price
// field, flags and a double, with the quantity in byte 11.
const tradeOfferRecordSize = 30

func tradeOfferPacket(offer game.TradeOffer) []byte {
	b := protocol.Builder{protocol.CommandTrade, protocol.TradeItemOffer}.U32(offer.Gold)
	for _, item := range offer.Items {
		record := make([]byte, tradeOfferRecordSize)
		record[0] = byte(item.Item.ID)
		record[1] = byte(item.Item.ID >> 8)
		// The native handler assembles the quantity from bytes 2, 3, 10 and 11;
		// writing it to both candidates keeps the row visible either way.
		record[2] = item.Count
		record[11] = item.Count
		b = b.Bytes(record)
	}
	return b.Bytes(make([]byte, 5))
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
		// Refresh both windows so the other side sees the confirmation state.
		for _, player := range run.players {
			s.tradeSend(player, tradeWindow(player, run.players[1-run.side(player)]))
		}
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
				s.tradeSend(player, tradeMessage("Trade failed: "+err.Error()+"."))
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
		s.tradeSend(player, packets...)
	}
	return nil
}

// tradeWindow is AC25:12, the native exchange window. The client reads a fixed
// 0x17-byte header and 0x51-byte records; padding keeps short windows safe.
func tradeWindow(c, partner *Session) []byte {
	selfGold, partnerGold := c.character.Gold, partner.character.Gold
	var records []game.TradeItem
	selfConfirmed, partnerConfirmed := uint16(0), uint16(0)
	if run := c.trade; run != nil && partner.trade == run {
		selfGold = run.offers[run.side(c)].Gold
		partnerGold = run.offers[run.side(partner)].Gold
		records = run.offers[run.side(partner)].Items
		// The header carries two confirmation flags: the viewer's own state and
		// the partner's. They are the fields the client shows next to each side.
		if run.accepted[run.side(c)] {
			selfConfirmed = 1
		}
		if run.accepted[run.side(partner)] {
			partnerConfirmed = 1
		}
	}
	flag := byte(0)
	if len(records) > 0 {
		flag = 1
	}
	b := protocol.Builder{protocol.CommandTrade, protocol.TradeWindow}.U32(partner.character.ID)
	b = b.U16(uint16(partner.character.ID)).U8(0).U32(selfGold).U32(partnerGold).U16(selfConfirmed).U16(partnerConfirmed).U8(0).U8(0).U8(flag).U8(0)
	// The client reads one 21-byte header per row and copies a single trailing
	// 0x51-byte structure after the loop, so bodies must not follow every row.
	for _, item := range records {
		b = b.Bytes(tradeRecordHead(item))
	}
	if len(records) > 0 {
		b = b.Bytes(tradeRecordBody(records[0]))
	}
	return b
}

// tradeRecordHead is the 11-field row header the client reads in its loop. The
// leading u16 is the key it looks up in the item table, so it carries the item
// id rather than the inventory slot.
func tradeRecordHead(item game.TradeItem) []byte {
	return protocol.Builder{}.U16(item.Item.ID).U8(item.Count).U8(item.Item.Damage).U8(100).U16(0).Bytes(make([]byte, 8)).U8(0).U8(0).U16(0).U8(0).U8(0)
}

// tradeRecordBody is the single 0x51-byte structure the client copies after the
// row loop; it describes the selected item.
func tradeRecordBody(item game.TradeItem) []byte {
	body := protocol.Builder{}.U16(item.Item.ID).U8(item.Count).U8(item.Item.Damage).U8(100).U8(100).U16(0).Bytes(make([]byte, 5)).U8(0)
	if len(body) < 0x51 {
		body = append(body, make([]byte, 0x51-len(body))...)
	}
	return body[:0x51]
}

// tradeLog prints one trade packet in the native dump format. Debug level only.
func (s *Server) tradeLog(dir string, c *Session, p []byte) {
	if s.Log == nil || len(p) < 2 || p[0] != protocol.CommandTrade {
		return
	}
	s.Log.Debug(fmt.Sprintf("trade %s session=%d code=0x%02x sub=0x%02x len=%d hex=% x", dir, c.info.ID, p[0], p[1], len(p), p))
}

func (s *Server) tradeSend(c *Session, packets ...[]byte) {
	for _, p := range packets {
		s.tradeLog("tx", c, p)
		s.sendOrClose(c, p)
	}
}
