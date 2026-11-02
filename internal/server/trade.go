package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
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

func tradeAvailable(c *Session) bool {
	return c != nil && c.ready && c.character.Preferences().TradeAllowed && c.battle == nil && c.event == nil && !c.storm && c.beach == nil && c.trade == nil && c.stall == nil
}
func tradeNear(a, b *Session) bool {
	return sameScene(a, b) && math.Hypot(float64(int(a.character.X)-int(b.character.X)), float64(int(a.character.Y)-int(b.character.Y))) <= tradeRangePixels
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
		return c.send(serverwire.TradeEnded(protocol.TradeResultAlreadyTrading))
	}
	target := s.mapPlayer(c, id)
	switch {
	case target == c:
		return nil
	case target == nil:
		for _, peer := range s.world {
			if peer.character.ID == id {
				return c.send(serverwire.TradeEnded(protocol.TradeResultDifferentMaps))
			}
		}
		return c.send(serverwire.TradeEnded(protocol.TradeResultDisconnected))
	case !tradeAvailable(c) || !tradeAvailable(target):
		return c.send(serverwire.TradeEnded(protocol.TradeResultBusy))
	case !tradeNear(c, target):
		return c.send(serverwire.TradeEnded(protocol.TradeResultUnavailable))
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
	s.sendOrClose(run.players[1-side], serverwire.TradeOfferPacket(offer))
	return nil
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
			dismount := serverwire.VehicleDismountPacket(&old[i])
			s.broadcastWorld(player, dismount)
			packets = append(packets, dismount)
		}
		for _, it := range run.offers[i].Items {
			packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, it.Slot, it.Count})
		}
		if len(result.Additions[i]) > 0 {
			packets = append(packets, player.character.Bag.AdditionPacket(result.Additions[i]))
		}
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(player.character.Gold), serverwire.TradeEnded(protocol.TradeResultCompleted))
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
		s.sendOrClose(c, serverwire.TradeEnded(own))
	}
	s.sendOrClose(other, serverwire.TradeEnded(partner))
}

const stallTitleMaxBytes = 64

type stallListing struct {
	Index byte
	Item  game.TradeItem
	Price uint32
}
type playerStall struct {
	Native      bool
	NativeSlots bool // Native five-byte offers use bag slots; the six-byte compatibility form uses listing indexes.
	Style       byte
	Title       string
	Listings    []stallListing
}

func stallSign(c *Session, title string) []byte {
	p, _ := protocol.Builder{protocol.CommandStall, protocol.StallSign}.U32(c.character.ID).String(title)
	return p
}
func (s *Server) closeStall(c *Session) {
	if c.stall == nil {
		return
	}
	native := c.stall.Native
	c.stall = nil
	p := stallSign(c, "")
	s.broadcastWorld(c, p)
	s.sendOrClose(c, p)
	if native {
		pose := protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(c.character.ID).U8(0)
		s.broadcastWorld(c, pose)
		s.sendOrClose(c, pose)
		gone := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallDespawn}.U32(c.character.ID)
		s.broadcastWorld(c, gone)
		s.sendOrClose(c, gone, []byte{protocol.CommandInventory, protocol.InventoryStallFormClose, protocol.StallItemShopType})
	}
}
func (s *Server) viewStall(c, seller *Session) error {
	if seller == nil || seller.stall == nil || !seller.ready || !tradeNear(c, seller) {
		return c.send(serverwire.TradeMessage("This stall is unavailable."))
	}
	p, _ := protocol.Builder{protocol.CommandStall, protocol.StallView}.U32(seller.character.ID).String(seller.stall.Title)
	p = p.U8(byte(len(seller.stall.Listings)))
	for _, row := range seller.stall.Listings {
		p = p.U8(row.Item.Slot).U16(row.Item.Item.ID).U32(row.Price).U8(row.Item.Count)
	}
	if seller.stall.Native {
		return s.sendAll(c, [][]byte{nativeStallBrowse(seller), p})
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
		next := &playerStall{Title: title}
		seen := map[byte]bool{}
		for range int(count) {
			slot, id, price, quantity := r.U8(), r.U16(), r.U32(), r.U8()
			if r.Err() != nil || slot < 1 || int(slot) > game.BagSize || seen[slot] || quantity == 0 || price == 0 || price > game.MaxGold {
				return protocol.ErrMalformed
			}
			seen[slot] = true
			item := c.character.Bag[slot-1]
			if item.Empty() || item.Locked || item.ID != id || quantity > item.Count {
				return c.send(serverwire.TradeMessage("The offered inventory has changed."))
			}
			if _, ok := s.Assets.Items[id]; !ok {
				return c.send(serverwire.TradeMessage("Unknown item."))
			}
			if c.character.ActiveVehicle == item.ID {
				return c.send(serverwire.TradeMessage("Dismount before selling this vehicle."))
			}
			next.Listings = append(next.Listings, stallListing{Index: byte(len(next.Listings)), Item: game.TradeItem{Slot: slot, Count: quantity, Item: item}, Price: price})
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		if !gmIdle(c) || !c.character.Preferences().TradeAllowed || c.invisible {
			return c.send(serverwire.TradeMessage("Finish active interactions before opening a stall."))
		}
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
			return c.send(serverwire.TradeMessage("This purchase is unavailable."))
		}
		index := -1
		for i, row := range seller.stall.Listings {
			if row.Item.Slot == slot {
				index = i
				break
			}
		}
		if index < 0 {
			return c.send(serverwire.TradeMessage("This item is no longer listed."))
		}
		row := seller.stall.Listings[index]
		total := uint64(row.Price) * uint64(count)
		if count > row.Item.Count || total > game.MaxGold {
			return c.send(serverwire.TradeMessage("Invalid purchase quantity."))
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
			return c.send(serverwire.TradeMessage("Purchase failed: " + err.Error() + "."))
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
		s.sendOrClose(c, c.character.Bag.AdditionPacket(result.Additions[1]), protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(c.character.Gold), serverwire.TradeMessage(fmt.Sprintf("Purchased %d item(s) for %d gold.", count, total)))
		if len(seller.stall.Listings) == 0 {
			s.closeStall(seller)
			return nil
		}
		return s.viewStall(c, seller)
	default:
		return ErrUnsupported
	}
}

const (
	nativeStallListingLimit             = 5 // Form_BuyItemStall_1, FUN_0033fa00.
	nativeStallOfferRecordBytes         = 5
	nativeStallCompatibilityRecordBytes = 6
	nativeStallDefaultStyle             = 1
	nativeStallMaxStyle                 = 14
	nativeStallVendorPose               = 1
	nativeStallPurchaseRecordBytes      = 8
	nativeStallDurabilityMax            = 100
)

func nativeStallMapState(c *Session) []byte {
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallMapState}.U32(c.character.ID).String(c.stall.Title)
	style := c.stall.Style
	if style < nativeStallDefaultStyle || style > nativeStallMaxStyle {
		style = nativeStallDefaultStyle
	}
	return p.U8(style)
}

func nativeStallBrowse(seller *Session) []byte {
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallPurchase}.U32(seller.character.ID).U32(0).U32(0).U32(0).U8(0)
	for i, row := range seller.stall.Listings {
		if i >= nativeStallListingLimit {
			break
		}
		item := row.Item.Item
		// 39-byte native browse records. Durable exchange retains the entire
		// instance; the known wire damage and forge fields describe that instance.
		p = p.U8(stallWireIndex(seller, row)).U16(item.ID).U8(row.Item.Count).U8(game.MaxItemStack).U32(row.Price).
			U8(byte(max(0, nativeStallDurabilityMax-int(item.Damage)))).U8(nativeStallDurabilityMax).
			U16(uint16(item.Metadata[game.ForgeMetadataOffset])).Bytes(make([]byte, 8)).U8(0).U8(item.Damage).
			U16(0).U8(0).U8(0).U32(0).U16(0).U32(0).U16(0)
	}
	return p
}

// Native requests adapt to the same authoritative pair exchange as AC56.
// Quotes must match the advertised item and unit price before any transfer.
func (s *Server) nativeStallCommand(ctx context.Context, c *Session, p []byte) error {
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.InventoryStallOpen:
		title, header := r.String(), r.U8()
		compatibility := header > 0 && header <= nativeStallListingLimit && r.Remaining() == int(header)*nativeStallCompatibilityRecordBytes
		count := int(header)
		if !compatibility {
			if r.Remaining() == 0 || r.Remaining()%nativeStallOfferRecordBytes != 0 {
				return protocol.ErrMalformed
			}
			count = r.Remaining() / nativeStallOfferRecordBytes
		}
		if r.Err() != nil || count == 0 || count > nativeStallListingLimit {
			return protocol.ErrMalformed
		}
		style := header
		if compatibility || style < nativeStallDefaultStyle || style > nativeStallMaxStyle {
			style = nativeStallDefaultStyle
		}
		legacy, err := protocol.Builder{protocol.CommandStall, protocol.StallOpen}.String(title)
		if err != nil {
			return err
		}
		legacy = legacy.U8(byte(count))
		for range count {
			slot := r.U8()
			if slot < 1 || slot > game.BagSize {
				return protocol.ErrMalformed
			}
			quantity := c.character.Bag[slot-1].Count
			if compatibility {
				quantity = r.U8()
			}
			price := r.U32()
			legacy = legacy.U8(slot).U16(c.character.Bag[slot-1].ID).U32(price).U8(quantity)
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		if err = s.stallCommand(ctx, c, legacy); err != nil {
			return err
		}
		if c.stall == nil {
			return nil
		}
		c.stall.Native, c.stall.NativeSlots, c.stall.Style = true, !compatibility, style
		confirm, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallOpen}.String(c.stall.Title)
		confirm = confirm.U8(style)
		for _, row := range c.stall.Listings {
			confirm = confirm.U8(row.Item.Slot).U32(row.Price)
		}
		state := nativeStallMapState(c)
		pose := protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(c.character.ID).U8(nativeStallVendorPose)
		s.broadcastWorld(c, pose)
		s.broadcastWorld(c, state)
		return s.sendAll(c, [][]byte{pose, confirm, state})
	case protocol.InventoryStallClose:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.stallCommand(ctx, c, []byte{protocol.CommandStall, protocol.StallClose})
	case protocol.InventoryStallBrowse:
		if len(p) != 6 {
			return protocol.ErrMalformed
		}
		id := r.U32()
		if id >= store.BazaarVendorFirstID {
			return s.browseBazaar(ctx, c, id)
		}
		seller := s.mapPlayer(c, id)
		if seller != nil && seller.stall != nil && !seller.stall.Native && seller.ready && tradeNear(c, seller) {
			seller.stall.NativeSlots = true
			return c.send(nativeStallBrowse(seller))
		}
		return s.viewStall(c, seller)
	case protocol.InventoryStallPurchase:
		if len(p) < 6+nativeStallPurchaseRecordBytes || (len(p)-6)%nativeStallPurchaseRecordBytes != 0 || (len(p)-6)/nativeStallPurchaseRecordBytes > nativeStallListingLimit {
			return protocol.ErrMalformed
		}
		vendor := r.U32()
		if vendor >= store.BazaarVendorFirstID {
			var orders []store.BazaarOrder
			for r.Remaining() > 0 {
				index, quantity, id, price := r.U8(), r.U8(), r.U16(), r.U32()
				orders = append(orders, store.BazaarOrder{Index: index, Count: quantity, ItemID: id, Price: price})
			}
			return s.buyBazaar(ctx, c, vendor, orders)
		}
		for r.Remaining() > 0 {
			index, quantity, id, price := r.U8(), r.U8(), r.U16(), r.U32()
			seller := s.mapPlayer(c, vendor)
			if seller == nil || seller.stall == nil {
				return c.send(serverwire.TradeMessage("This stall is unavailable."))
			}
			found := false
			var slot byte
			for _, row := range seller.stall.Listings {
				if stallWireIndex(seller, row) == index && row.Item.Item.ID == id && row.Price == price {
					found, slot = true, row.Item.Slot
					break
				}
			}
			if !found || quantity == 0 {
				return c.send(serverwire.TradeMessage("The stall quote has changed."))
			}
			if err := s.stallCommand(ctx, c, protocol.Builder{protocol.CommandStall, protocol.StallBuy}.U32(vendor).U8(slot).U8(quantity)); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrUnsupported
}

func stallWireIndex(seller *Session, row stallListing) byte {
	if seller.stall.NativeSlots {
		return row.Item.Slot
	}
	return row.Index
}

// stallPacket keeps a seller's own read/close requests from auto-closing the sign.
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
	case protocol.InventoryStallOpen, protocol.InventoryStallClose, protocol.InventoryStallBrowse, protocol.InventoryStallPurchase:
		return true
	}
	return false
}

// stallList completes native map synchronization even when no stalls are open.
func (s *Server) stallList(c *Session) [][]byte {
	var packets [][]byte
	for _, peer := range s.peers(c) {
		if peer.invisible || peer.stall == nil {
			continue
		}
		packets = append(packets, nativeStallMapState(peer), stallSign(peer, peer.stall.Title))
		if peer.stall.Native {
			packets = append(packets, protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(peer.character.ID).U8(nativeStallVendorPose))
		}
	}
	return append(packets, []byte{protocol.CommandStall, protocol.StallSign, 0})
}

const (
	personalStallMinimumPrice  = 50
	personalStallPricePerLevel = 100
)

func (s *Server) openAutomaticStall(ctx context.Context, c *Session, title string) error {
	if !gmIdle(c) {
		return nil
	}
	var rows []stallListing
	for i, item := range c.character.Bag {
		definition, known := s.Assets.Items[item.ID]
		if item.Empty() || item.Locked || !known || c.character.ActiveVehicle != 0 && c.character.VehicleSlot == byte(i+1) {
			continue
		}
		price := uint32(max(personalStallMinimumPrice, max(1, int(definition.Level))*personalStallPricePerLevel))
		rows = append(rows, stallListing{Item: game.TradeItem{Slot: byte(i + 1), Count: item.Count}, Price: price})
		if len(rows) == nativeStallListingLimit {
			break
		}
	}
	if len(rows) == 0 {
		return s.chatFeedback(c, "There are no unlocked owned items to list.")
	}
	p, err := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallOpen}.String(title)
	if err != nil {
		return err
	}
	p = p.U8(byte(len(rows)))
	for _, row := range rows {
		p = p.U8(row.Item.Slot).U8(row.Item.Count).U32(row.Price)
	}
	return s.nativeStallCommand(ctx, c, p)
}
func (s *Server) browseAnyStall(ctx context.Context, c *Session, id uint32) error {
	if !gmIdle(c) {
		return nil
	}
	if id >= store.BazaarVendorFirstID {
		return s.browseBazaar(ctx, c, id)
	}
	return s.viewStall(c, s.mapPlayer(c, id))
}
func (s *Server) buyStallIndex(ctx context.Context, c *Session, id uint32, index, count byte) error {
	if !gmIdle(c) || count == 0 {
		return nil
	}
	if id >= store.BazaarVendorFirstID {
		quote, known := c.bazaarQuotes[id]
		if !known {
			return s.chatFeedback(c, "Browse this bazaar before purchasing.")
		}
		i := slices.IndexFunc(quote.Listings, func(row store.BazaarListing) bool { return row.Index == index })
		if i < 0 {
			return s.chatFeedback(c, "This listing is unavailable.")
		}
		row := quote.Listings[i]
		return s.buyBazaar(ctx, c, id, []store.BazaarOrder{{Index: index, Count: count, ItemID: row.ItemID, Price: row.Price}})
	}
	seller := s.mapPlayer(c, id)
	if seller == nil || seller.stall == nil {
		return s.chatFeedback(c, "This stall is unavailable.")
	}
	for _, row := range seller.stall.Listings {
		if row.Index == index {
			return s.stallCommand(ctx, c, protocol.Builder{protocol.CommandStall, protocol.StallBuy}.U32(id).U8(row.Item.Slot).U8(count))
		}
	}
	return s.chatFeedback(c, "This listing is unavailable.")
}
func (s *Server) bazaarCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.BazaarOpen:
		title := "Player Shop"
		if r.Remaining() > 0 {
			title = r.String()
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		return s.openAutomaticStall(ctx, c, title)
	case protocol.BazaarBuy:
		if len(p) != 7 && len(p) != 9 {
			return protocol.ErrMalformed
		}
		id, index, count := r.U32(), r.U8(), uint16(1)
		if r.Remaining() > 0 {
			count = r.U16()
		}
		if count == 0 || count > game.MaxItemStack {
			return protocol.ErrMalformed
		}
		return s.buyStallIndex(ctx, c, id, index, byte(count))
	case protocol.BazaarClose:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		s.closeStall(c)
		return nil
	case protocol.BazaarQuery:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		bs, err := s.Store.Bazaars(ctx, 0)
		if err != nil {
			return err
		}
		count := len(bs)
		for _, peer := range s.world {
			if peer.stall != nil {
				count++
			}
		}
		return c.send(protocol.Builder{protocol.CommandBazaar, protocol.BazaarQuery}.U16(uint16(min(count, math.MaxUint16))))
	}
	return ErrUnsupported
}
func (s *Server) eventStallCommand(ctx context.Context, c *Session, p []byte) error {
	switch p[1] {
	case protocol.EventStallOpen:
		next := append([]byte(nil), p...)
		next[0], next[1] = protocol.CommandBazaar, protocol.BazaarOpen
		return s.bazaarCommand(ctx, c, next)
	case protocol.EventStallClose:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		s.closeStall(c)
		return nil
	case protocol.EventStallBrowse:
		if len(p) != 6 {
			return protocol.ErrMalformed
		}
		return s.browseAnyStall(ctx, c, protocol.NewReader(p[2:]).U32())
	case protocol.EventStallBuy:
		if len(p) != 7 && len(p) != 8 {
			return protocol.ErrMalformed
		}
		r := protocol.NewReader(p[2:])
		id, index, count := r.U32(), r.U8(), byte(1)
		if r.Remaining() > 0 {
			count = r.U8()
		}
		return s.buyStallIndex(ctx, c, id, index, count)
	}
	return ErrUnsupported
}

const (
	bazaarActorLevel           = 50
	bazaarActorHair            = 44828
	bazaarActorSkin            = 6781
	bazaarActorOutfit          = 21004
	bazaarActorBoots           = 24004
	bazaarPetEquipmentBytes    = 126
	bazaarPetNameStatsBytes    = 81
	bazaarPetNameBytes         = 16
	bazaarVehicleHeaderPadding = 11
)

func bazaarNear(c *Session, b store.Bazaar) bool {
	return c.tentOwner == 0 && c.character.Map == b.Map && math.Hypot(float64(int(c.character.X)-int(b.X)), float64(int(c.character.Y)-int(b.Y))) <= store.BazaarRangePixels
}
func bazaarActor(b store.Bazaar) game.Character {
	return game.Character{ID: b.VendorID, Slot: 1, Name: b.Name, Body: game.BodyBigMale, Head: 0, Level: bazaarActorLevel, Element: game.Earth, HP: 1, MaxHP: 1, Map: b.Map, X: b.X, Y: b.Y, Color1: bazaarActorHair | bazaarActorSkin<<16, Color2: bazaarActorHair | bazaarActorSkin<<16, Equipment: game.Equipment{{ID: bazaarActorOutfit, Count: 1}, {ID: bazaarActorBoots, Count: 1}}}
}
func bazaarPackets(b store.Bazaar) [][]byte {
	actor := bazaarActor(b)
	appearance, _ := actor.AppearancePacket(true)
	state, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallMapState}.U32(b.VendorID).String(b.Title)
	return [][]byte{appearance, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateEquipmentSnapshot}.U32(b.VendorID).U16(bazaarActorOutfit).U16(bazaarActorBoots), actor.PositionPacket(), state.U8(b.Kind)}
}
func (s *Server) syncBazaars(ctx context.Context, c *Session) error {
	if c.tentOwner != 0 {
		return nil
	}
	bs, err := s.Store.Bazaars(ctx, c.character.Map)
	if err != nil {
		return err
	}
	for _, b := range bs {
		if err = s.sendAll(c, bazaarPackets(b)); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) bazaarBrowsePacket(b store.Bazaar) []byte {
	if b.Kind == store.BazaarItems {
		listings := make([]stallListing, 0, len(b.Listings))
		for _, row := range b.Listings {
			if row.Quantity == 0 && !row.Infinite && !b.Infinite {
				continue
			}
			listings = append(listings, stallListing{Index: row.Index, Item: game.TradeItem{Item: game.Item{ID: row.ItemID, Count: 1}, Count: byte(min(uint32(game.MaxItemStack), row.Quantity))}, Price: row.Price})
		}
		return nativeStallBrowse(&Session{character: &game.Character{ID: b.VendorID}, stall: &playerStall{Listings: listings}})
	}
	if b.Kind == store.BazaarVehicles {
		p := protocol.Builder{protocol.CommandInventory, protocol.InventoryVehicleStallBrowse}.U32(b.VendorID).Bytes(make([]byte, bazaarVehicleHeaderPadding))
		for _, row := range b.Listings {
			if row.Quantity == 0 && !row.Infinite && !b.Infinite {
				continue
			}
			p = p.U8(row.Index + 1).U16(row.ItemID).U8(nativeStallDurabilityMax).U8(nativeStallDurabilityMax).U32(row.Price).U32(0).U16(0)
		}
		return p
	}
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryPetStallBrowse}.U32(b.VendorID)
	for _, row := range b.Listings {
		if row.Quantity == 0 && !row.Infinite && !b.Infinite {
			continue
		}
		pet := game.NewPet(uint32(row.ItemID), s.npcName(uint32(row.ItemID)), 1, s.petTemplate(uint32(row.ItemID)), s.Assets.Items)
		template := s.Assets.NPCs[row.ItemID]
		p = p.U8(row.Index).U16(row.ItemID).U32(row.Price).U8(pet.Level).U32(uint32(pet.HP)).U32(uint32(pet.SP)).U16(uint16(template.Element)).Bytes(make([]byte, bazaarPetEquipmentBytes))
		stats := make([]byte, bazaarPetNameStatsBytes)
		copy(stats[:bazaarPetNameBytes], []byte(s.npcName(uint32(row.ItemID))))
		stats[bazaarPetNameBytes] = byte(pet.Base.Strength)
		stats[bazaarPetNameBytes+1] = byte(pet.Base.Constitution)
		stats[bazaarPetNameBytes+2] = byte(pet.Base.Intelligence)
		stats[bazaarPetNameBytes+3] = byte(pet.Base.Wisdom)
		stats[bazaarPetNameBytes+4] = byte(pet.Base.Agility)
		stats[bazaarPetNameBytes+5] = pet.Amity
		p = p.Bytes(stats)
	}
	return p
}
func (s *Server) browseBazaar(ctx context.Context, c *Session, id uint32) error {
	b, err := s.Store.Bazaar(ctx, id)
	if err != nil || !bazaarNear(c, b) {
		return s.chatFeedback(c, "This bazaar is unavailable.")
	}
	if c.bazaarQuotes == nil {
		c.bazaarQuotes = map[uint32]store.Bazaar{}
	}
	c.bazaarQuotes[id] = b
	return c.send(s.bazaarBrowsePacket(b))
}
func (s *Server) buyBazaar(ctx context.Context, c *Session, id uint32, orders []store.BazaarOrder) error {
	if !gmIdle(c) {
		return nil
	}
	quote, known := c.bazaarQuotes[id]
	if !known || !bazaarNear(c, quote) {
		return s.chatFeedback(c, "Browse this bazaar before purchasing.")
	}
	pets := map[uint16]game.PetTemplate{}
	for _, order := range orders {
		i := slices.IndexFunc(quote.Listings, func(row store.BazaarListing) bool { return row.Index == order.Index })
		if i < 0 || quote.Listings[i].ItemID != order.ItemID || quote.Listings[i].Price != order.Price {
			return s.chatFeedback(c, "The bazaar quote has changed.")
		}
		if _, ok := s.Assets.NPCs[order.ItemID]; ok {
			pets[order.ItemID] = s.petTemplate(uint32(order.ItemID))
		}
	}
	before := c.character.Clone()
	next, b, adds, err := s.Store.BuyBazaar(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, id, orders, s.Assets.Items, pets, *c.character)
	if err != nil {
		return s.chatFeedback(c, "Bazaar purchase failed: "+err.Error())
	}
	s.adoptSavedCharacter(c, next)
	c.bazaarQuotes[id] = b
	s.sendOrClose(c, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
	if len(adds) > 0 {
		s.sendOrClose(c, c.character.Bag.AdditionPacket(adds))
	}
	for _, pet := range next.Pets {
		if _, owned := before.Pet(pet.ID); owned {
			continue
		}
		c.pets.register(pet.ID)
		s.sendOrClose(c, pet.RecruitPacket(next.ID, s.petTemplate(pet.ID)))
		s.sendOrClose(c, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items)...)
		s.sendOrClose(c, serverwire.PetNamePacket(next.ID, c.pets.slot(pet.ID), s.companionName(pet.ID, pet.Name)))
	}
	s.sendOrClose(c, s.bazaarBrowsePacket(b))
	return nil
}
func (s *Server) bazaarTypedPurchase(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 7 {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	id := r.U32()
	quote, known := c.bazaarQuotes[id]
	if !known {
		return s.chatFeedback(c, "Browse this bazaar before purchasing.")
	}
	kind := byte(store.BazaarPets)
	stride := 1
	if p[1] == protocol.InventoryVehicleStallPurchase {
		kind = store.BazaarVehicles
		stride = 2
	}
	if quote.Kind != kind || r.Remaining()%stride != 0 || r.Remaining()/stride > store.BazaarPetListingLimit {
		return protocol.ErrMalformed
	}
	var orders []store.BazaarOrder
	for r.Remaining() > 0 {
		index, count := r.U8(), byte(1)
		if stride == 2 {
			if index == 0 {
				return protocol.ErrMalformed
			}
			index--
			count = r.U8()
		}
		i := slices.IndexFunc(quote.Listings, func(row store.BazaarListing) bool { return row.Index == index })
		if i < 0 {
			return s.chatFeedback(c, "The bazaar quote has changed.")
		}
		row := quote.Listings[i]
		orders = append(orders, store.BazaarOrder{Index: index, Count: count, ItemID: row.ItemID, Price: row.Price})
	}
	return s.buyBazaar(ctx, c, id, orders)
}
