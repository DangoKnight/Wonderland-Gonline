package server

import (
	"context"
	"math"
	"slices"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

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
