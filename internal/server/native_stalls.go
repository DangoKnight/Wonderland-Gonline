package server

import (
	"context"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

const (
	nativeStallListingLimit        = 5 // Form_BuyItemStall_1, FUN_0033fa00.
	nativeStallPurchaseRecordBytes = 8
	nativeStallDurabilityMax       = 100
)

func nativeStallMapState(c *Session) []byte {
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallMapState}.U32(c.character.ID).String(c.stall.Title)
	return p.U8(protocol.StallItemShopType)
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
		p = p.U8(row.Index).U16(item.ID).U8(row.Item.Count).U8(game.MaxItemStack).U32(row.Price).
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
		title, count := r.String(), r.U8()
		if r.Err() != nil || count == 0 || count > nativeStallListingLimit {
			return protocol.ErrMalformed
		}
		legacy, err := protocol.Builder{protocol.CommandStall, protocol.StallOpen}.String(title)
		if err != nil {
			return err
		}
		legacy = legacy.U8(count)
		for range int(count) {
			slot, quantity, price := r.U8(), r.U8(), r.U32()
			if slot < 1 || slot > game.BagSize {
				return protocol.ErrMalformed
			}
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
		c.stall.Native = true
		confirm, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallOpen}.String(c.stall.Title)
		confirm = confirm.U8(byte(len(c.stall.Listings)))
		for _, row := range c.stall.Listings {
			confirm = confirm.U8(row.Item.Slot).U32(row.Price)
		}
		state := nativeStallMapState(c)
		s.broadcastWorld(c, state)
		return s.sendAll(c, [][]byte{confirm, state})
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
		return s.viewStall(c, s.mapPlayer(c, id))
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
				if row.Index == index && row.Item.Item.ID == id && row.Price == price {
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
