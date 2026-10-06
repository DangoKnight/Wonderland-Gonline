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
	p := stallSign(c, "")
	s.broadcastWorld(c, p)
	s.sendOrClose(c, p)
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
