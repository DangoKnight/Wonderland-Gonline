package server

import (
	"context"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

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
				return c.send(tradeMessage("This stall is unavailable."))
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
				return c.send(tradeMessage("The stall quote has changed."))
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
