package server

import (
	"context"
	"fmt"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

const stallTitleMaxBytes = 64

type stallListing struct {
	Item  game.TradeItem
	Price uint32
}
type playerStall struct {
	Title    string
	Style    byte
	Listings []stallListing
}

func stallSign(c *Session, title string) []byte {
	p, _ := protocol.Builder{protocol.CommandStall, protocol.StallSign}.U32(c.character.ID).String(title)
	return p
}
func (s *Server) closeStall(c *Session) {
	if c.stall == nil {
		return
	}
	c.stall = nil
	s.sendStallRemoval(c)
}

// sendStallRemoval tears the native table down for peers and the vendor.
func (s *Server) sendStallRemoval(c *Session) {
	pose := protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(c.character.ID).U8(0)
	despawn := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallDespawn}.U32(c.character.ID)
	sign := stallSign(c, "")
	s.broadcastWorld(c, pose)
	s.broadcastWorld(c, despawn)
	s.broadcastWorld(c, sign)
	s.sendOrClose(c, pose)
	s.sendOrClose(c, despawn)
	s.sendOrClose(c, sign)
}
func (s *Server) viewStall(c, seller *Session) error {
	if seller == nil || seller.stall == nil || !seller.ready || !tradeNear(c, seller) {
		return c.send(tradeMessage("This stall is unavailable."))
	}
	p, _ := protocol.Builder{protocol.CommandStall, protocol.StallView}.U32(seller.character.ID).String(seller.stall.Title)
	p = p.U8(byte(len(seller.stall.Listings)))
	for _, row := range seller.stall.Listings {
		p = p.U8(row.Item.Slot).U16(row.Item.Item.ID).U32(row.Price).U8(row.Item.Count)
	}
	return c.send(p)
}

// Stalls are session-owned signs and offers; inventory and currency remain in SQL.
// Every purchase revalidates the exact offered record inside the pair transaction.
func (s *Server) stallCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < protocol.StallHeaderRequestBytes {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.StallOpen:
		title, count := r.String(), r.U8()
		if !validChatText(title, stallTitleMaxBytes) || count == 0 || int(count) > game.BagSize {
			return protocol.ErrMalformed
		}
		next := &playerStall{Title: title, Style: 1}
		seen := map[byte]bool{}
		for range int(count) {
			slot, id, price, quantity := r.U8(), r.U16(), r.U32(), r.U8()
			if r.Err() != nil || slot < 1 || int(slot) > game.BagSize || seen[slot] || quantity == 0 || price == 0 || price > game.MaxGold {
				return protocol.ErrMalformed
			}
			seen[slot] = true
			item := c.character.Bag[slot-1]
			if item.Empty() || item.Locked || item.ID != id || quantity > item.Count {
				return c.send(tradeMessage("The offered inventory has changed."))
			}
			if _, ok := s.Assets.Items[id]; !ok {
				return c.send(tradeMessage("Unknown item."))
			}
			if c.character.ActiveVehicle == item.ID {
				return c.send(tradeMessage("Dismount before selling this vehicle."))
			}
			next.Listings = append(next.Listings, stallListing{Item: game.TradeItem{Slot: slot, Count: quantity, Item: item}, Price: price})
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		if !gmIdle(c) || !c.character.Preferences().TradeAllowed || c.invisible {
			return c.send(tradeMessage("Finish active interactions before opening a stall."))
		}
		s.clearTradeRequests(c)
		c.gathering = nil
		c.stall = next
		sign := stallSign(c, title)
		s.broadcastWorld(c, sign)
		return c.send(sign)
	case protocol.StallClose:
		if len(p) != protocol.StallHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		s.closeStall(c)
		return nil
	case protocol.StallView:
		if len(p) != protocol.StallTargetRequestBytes {
			return protocol.ErrMalformed
		}
		return s.viewStall(c, s.mapPlayer(c, r.U32()))
	case protocol.StallBuy:
		if len(p) != protocol.StallPurchaseRequestBytes {
			return protocol.ErrMalformed
		}
		seller, slot, count := s.mapPlayer(c, r.U32()), r.U8(), r.U8()
		if count == 0 || seller == c || seller == nil || seller.stall == nil || c.stall != nil || !tradeAvailable(c) || !tradeNear(c, seller) || !seller.ready || seller.battle != nil || seller.event != nil || seller.storm || seller.beach != nil || seller.trade != nil {
			return c.send(tradeMessage("This purchase is unavailable."))
		}
		index := -1
		for i, row := range seller.stall.Listings {
			if row.Item.Slot == slot {
				index = i
				break
			}
		}
		if index < 0 {
			return c.send(tradeMessage("This item is no longer listed."))
		}
		row := seller.stall.Listings[index]
		total := uint64(row.Price) * uint64(count)
		if count > row.Item.Count || total > game.MaxGold {
			return c.send(tradeMessage("Invalid purchase quantity."))
		}
		offered := row.Item
		offered.Count = count
		var result game.TradeResult
		err := s.Store.UpdateCharacterPair(ctx, store.CharacterRef{Account: seller.account.ID, ID: seller.character.ID}, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(a, b *game.Character) error {
			var err error
			if err = game.PreserveItemLocks(*seller.character, a); err != nil {
				return err
			}
			if err = game.PreserveItemLocks(*c.character, b); err != nil {
				return err
			}
			result, err = game.Exchange(*a, *b, [2]game.TradeOffer{{Items: []game.TradeItem{offered}}, {Gold: uint32(total)}}, s.Assets.Items)
			if err == nil {
				*a, *b = result.Characters[0], result.Characters[1]
			}
			return err
		})
		if err != nil {
			return c.send(tradeMessage("Purchase failed: " + err.Error() + "."))
		}
		s.adoptSavedCharacter(seller, result.Characters[0])
		s.adoptSavedCharacter(c, result.Characters[1])
		row.Item.Count -= count
		row.Item.Item = seller.character.Bag[slot-1]
		if row.Item.Count == 0 {
			seller.stall.Listings = append(seller.stall.Listings[:index], seller.stall.Listings[index+1:]...)
		} else {
			seller.stall.Listings[index] = row
		}
		s.sendOrClose(seller, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, count}, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(seller.character.Gold))
		s.sendOrClose(c, c.character.Bag.AdditionPacket(result.Additions[1]), protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(c.character.Gold), tradeMessage(fmt.Sprintf("Purchased %d item(s) for %d gold.", count, total)))
		if len(seller.stall.Listings) == 0 {
			s.closeStall(seller)
			return nil
		}
		return s.viewStall(c, seller)
	default:
		return ErrUnsupported
	}
}

// nativeStallOpen is AC23:30: title, one style/count byte, then slot+price
// records. The client sends 5-byte records without quantities; each listing
// offers the whole stack in that bag slot.
func (s *Server) nativeStallOpen(ctx context.Context, c *Session, p []byte) error {
	r := protocol.NewReader(p[2:])
	title := r.String()
	style := r.U8()
	if r.Err() != nil || title == "" || len(title) > stallTitleMaxBytes || r.Remaining() == 0 || r.Remaining()%5 != 0 {
		return protocol.ErrMalformed
	}
	count := r.Remaining() / 5
	if count > 10 {
		return protocol.ErrMalformed
	}
	if style == 0 || style > 14 {
		style = 1
	}
	next := &playerStall{Title: title, Style: style}
	seen := map[byte]bool{}
	for range count {
		slot, price := r.U8(), r.U32()
		if r.Err() != nil || slot < 1 || int(slot) > game.BagSize || seen[slot] || price == 0 || price > game.MaxGold {
			return protocol.ErrMalformed
		}
		seen[slot] = true
		item := c.character.Bag[slot-1]
		if item.Empty() || item.Locked || item.Count == 0 {
			return c.send(tradeMessage("The offered inventory has changed."))
		}
		if _, ok := s.Assets.Items[item.ID]; !ok {
			return c.send(tradeMessage("Unknown item."))
		}
		next.Listings = append(next.Listings, stallListing{Item: game.TradeItem{Slot: slot, Count: item.Count, Item: item}, Price: price})
	}
	if !gmIdle(c) || !c.character.Preferences().TradeAllowed || c.invisible {
		return c.send(tradeMessage("Finish active interactions before opening a stall."))
	}
	s.clearTradeRequests(c)
	c.gathering = nil
	c.stall = next
	confirm, err := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallOpen}.String(title)
	if err != nil {
		return err
	}
	confirm = confirm.U8(style)
	for _, row := range next.Listings {
		confirm = confirm.U8(row.Item.Slot).U32(row.Price)
	}
	state, err := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallState}.U32(c.character.ID).String(title)
	if err != nil {
		return err
	}
	state = state.U8(style)
	pose := protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(c.character.ID).U8(1)
	sign := stallSign(c, title)
	s.broadcastWorld(c, pose)
	s.broadcastWorld(c, state)
	s.broadcastWorld(c, sign)
	return s.sendAll(c, [][]byte{pose, confirm, state, sign})
}

// nativeStallClose is AC23:31 from the vendor: tear down the native table.
func (s *Server) nativeStallClose(c *Session) error {
	c.stall = nil
	s.sendStallRemoval(c)
	return nil
}

// stallList is the native 38:1E snapshot: one record per open stall in the
// scene, then a zero-length terminator.
func (s *Server) stallList(c *Session) [][]byte {
	var out [][]byte
	seen := map[uint32]bool{}
	for _, peer := range s.world {
		if peer == c || peer.stall == nil || !peer.ready || peer.character.Map != c.character.Map || !s.samePlayerScene(peer, c) || seen[peer.character.ID] {
			continue
		}
		seen[peer.character.ID] = true
		style := peer.stall.Style
		if style == 0 {
			style = 1
		}
		state, err := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallState}.U32(peer.character.ID).String(peer.stall.Title)
		if err != nil {
			continue
		}
		out = append(out, state.U8(style), stallSign(peer, peer.stall.Title))
	}
	return append(out, []byte{protocol.CommandStall, protocol.StallSign, 0})
}

// nativeStallBrowse is AC23:32: the buyer names a vendor, and the client gets
// AC23:33 with up to five 39-byte merchandise records.
func (s *Server) nativeStallBrowse(c *Session, p []byte) error {
	if len(p) != 6 {
		return protocol.ErrMalformed
	}
	id := uint32(p[2]) | uint32(p[3])<<8 | uint32(p[4])<<16 | uint32(p[5])<<24
	seller := s.mapPlayer(c, id)
	if seller == nil || seller.stall == nil || !seller.ready || !tradeNear(c, seller) {
		return c.send(tradeMessage("This stall is unavailable."))
	}
	b := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallBrowseResult}.U32(id).U32(0).U32(0).U32(0).U8(0)
	for i, row := range seller.stall.Listings {
		if i >= 5 {
			break
		}
		b = b.U8(row.Item.Slot).U16(row.Item.Item.ID).U8(row.Item.Count).U8(50).U32(row.Price)
		b = b.U8(100).U8(100).U16(0).Bytes(make([]byte, 8)).U8(0).U8(0).U16(0).U8(0).U8(0).U32(0).U16(0).U32(0).U16(0)
	}
	return c.send(b)
}

// stallPacket reports the native stall subcommands that keep their own stall.
func stallPacket(p []byte) bool {
	if len(p) < 2 {
		return false
	}
	if p[0] == protocol.CommandStall {
		return true
	}
	if p[0] != protocol.CommandInventory {
		return false
	}
	switch p[1] {
	case protocol.InventoryStallOpen, protocol.InventoryStallClose, protocol.InventoryStallBrowse, protocol.InventoryStallBrowseResult:
		return true
	}
	return false
}

// nativeStallBuy is AC23:33 from a buyer: actor, slot, quantity, item id and
// a trailing gold field. The whole exchange commits atomically across both
// characters before any packet is sent.
func (s *Server) nativeStallBuy(ctx context.Context, c *Session, p []byte) error {
	if len(p) != 14 {
		return protocol.ErrMalformed
	}
	actor := uint32(p[2]) | uint32(p[3])<<8 | uint32(p[4])<<16 | uint32(p[5])<<24
	slot, qty := p[6], p[7]
	itemID := uint16(p[8]) | uint16(p[9])<<8
	if qty == 0 {
		qty = 1
	}
	seller := s.mapPlayer(c, actor)
	if seller == nil || seller == c || seller.stall == nil || !seller.ready || !tradeNear(c, seller) {
		return c.send(tradeMessage("This stall is unavailable."))
	}
	index := -1
	for i, row := range seller.stall.Listings {
		if row.Item.Slot == slot && row.Item.Item.ID == itemID {
			index = i
			break
		}
	}
	if index < 0 {
		return c.send(tradeMessage("This item is no longer listed."))
	}
	row := seller.stall.Listings[index]
	if qty > row.Item.Count {
		return c.send(tradeMessage("This item is out of stock."))
	}
	total := uint64(row.Price) * uint64(qty)
	if total > game.MaxGold || uint64(c.character.Gold) < total {
		return c.send(tradeMessage("Not enough gold."))
	}
	sellerOffer := game.TradeOffer{Items: []game.TradeItem{{Slot: row.Item.Slot, Count: qty, Item: row.Item.Item}}}
	buyerOffer := game.TradeOffer{Gold: uint32(total)}
	var result game.TradeResult
	err := s.Store.UpdateCharacterPair(ctx, store.CharacterRef{Account: seller.account.ID, ID: seller.character.ID}, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(first, second *game.Character) error {
		var err error
		result, err = game.Exchange(*first, *second, [2]game.TradeOffer{sellerOffer, buyerOffer}, s.Assets.Items)
		if err != nil {
			return err
		}
		*first, *second = result.Characters[0], result.Characters[1]
		return nil
	})
	if err != nil {
		return c.send(tradeMessage("Purchase failed: " + err.Error() + "."))
	}
	s.adoptSavedCharacter(seller, result.Characters[0])
	s.adoptSavedCharacter(c, result.Characters[1])
	seller.stall.Listings[index].Item.Count -= qty
	remaining := seller.stall.Listings[index].Item.Count
	if remaining <= 0 {
		seller.stall.Listings = append(seller.stall.Listings[:index], seller.stall.Listings[index+1:]...)
	}
	var sellerPackets, buyerPackets [][]byte
	sellerPackets = append(sellerPackets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, qty}, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(seller.character.Gold))
	buyerPackets = append(buyerPackets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(c.character.Gold))
	if len(result.Additions[1]) > 0 {
		buyerPackets = append(buyerPackets, c.character.Bag.AdditionPacket(result.Additions[1]))
	}
	buyerPackets = append(buyerPackets, s.stallBrowsePacket(c, seller)...)
	if len(seller.stall.Listings) == 0 {
		s.closeStall(seller)
		s.sendOrClose(seller, tradeMessage("Your stall is sold out and has been closed."))
	}
	if err := s.sendAll(seller, sellerPackets); err != nil {
		return err
	}
	return s.sendAll(c, buyerPackets)
}

// stallBrowsePacket builds AC23:33 for one buyer.
func (s *Server) stallBrowsePacket(c *Session, seller *Session) [][]byte {
	if seller == nil || seller.stall == nil {
		return nil
	}
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallBrowseResult}.U32(seller.character.ID).U32(0).U32(0).U32(0).U8(0)
	for i, row := range seller.stall.Listings {
		if i >= 5 {
			break
		}
		p = p.U8(row.Item.Slot).U16(row.Item.Item.ID).U8(row.Item.Count).U8(50).U32(row.Price)
		p = p.U8(100).U8(100).U16(0).Bytes(make([]byte, 8)).U8(0).U8(0).U16(0).U8(0).U8(0).U32(0).U16(0).U32(0).U16(0)
	}
	return [][]byte{p}
}
