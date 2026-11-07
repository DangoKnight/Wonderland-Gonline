package server

import (
	"context"
	"encoding/binary"
	"errors"
	"sort"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

type pendingItemCard struct {
	Slot byte
	Item game.Item
}

func (s *Server) cardDetails(id uint16, card assets.ItemCard) protocol.GiftCard {
	c := protocol.GiftCard{ID: id, Version: protocol.GiftCardVersion}
	for _, r := range card.Rewards {
		c.Rewards = append(c.Rewards, protocol.GiftCardReward{ItemID: r.ItemID, Quantity: r.Quantity})
	}
	return c
}
func (s *Server) sendCardIndex(c *Session) error {
	ids := make([]int, 0, len(s.Assets.ItemCards))
	for id := range s.Assets.ItemCards {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	if len(ids) == 0 {
		return nil
	}
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryGiftCard, protocol.GiftCardIndex}.U16(uint16(len(ids)))
	for _, id := range ids {
		p = p.U16(uint16(id)).U8(protocol.GiftCardVersion)
	}
	return c.send(p)
}
func (s *Server) cardCatalog(c *Session, p []byte) error {
	if len(p) < 4 || p[1] != protocol.GiftCardIndex {
		return protocol.ErrMalformed
	}
	n := int(binary.LittleEndian.Uint16(p[2:4]))
	if n == 0 || len(p) != 4+2*n {
		return protocol.ErrMalformed
	}
	cards := make([]protocol.GiftCard, 0, n)
	for i := 0; i < n; i++ {
		id := binary.LittleEndian.Uint16(p[4+i*2:])
		if card, ok := s.Assets.ItemCards[id]; ok {
			cards = append(cards, s.cardDetails(id, card))
		}
	}
	if len(cards) == 0 {
		return nil
	}
	return c.send(protocol.GiftCardDetailsPacket(cards))
}
func (s *Server) redeemItemCard(ctx context.Context, c *Session, slot byte) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	card, ok := s.Assets.ItemCards[item.ID]
	if !ok {
		if s.Assets.Items[item.ID].Type == game.ItemTypeRedemptionCard {
			return true, c.send(serverwire.HeadBanner("This card's rewards are unavailable. Card retained."))
		}
		return false, nil
	}
	if item.Empty() || item.Locked || c.event != nil || c.battle != nil || c.storm || c.beach != nil {
		return true, nil
	}
	c.itemCard = &pendingItemCard{slot, item}
	if card.MaleItem != 0 {
		id := card.MaleItem
		if c.character.Body == game.BodySmallFemale || c.character.Body == game.BodyBigFemale {
			id = card.FemaleItem
		}
		for i, r := range card.Rewards {
			if r.ItemID == id {
				return true, s.exchangeItemCard(ctx, c, byte(i+1), id)
			}
		}
		return true, nil
	}
	if len(card.Rewards) == 1 {
		return true, s.exchangeItemCard(ctx, c, 1, card.Rewards[0].ItemID)
	}
	return true, s.sendAll(c, [][]byte{protocol.GiftCardDetailsPacket([]protocol.GiftCard{s.cardDetails(item.ID, card)}), {protocol.CommandInventory, protocol.InventoryGiftCard, protocol.GiftCardOpen, slot}})
}
func (s *Server) itemCardCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 3 {
		return protocol.ErrMalformed
	}
	switch p[2] {
	case protocol.GiftCardIndex:
		if len(p) != 7 || p[3] < 1 || p[3] > game.BagSize {
			return protocol.ErrMalformed
		}
		if binary.LittleEndian.Uint16(p[4:6]) != c.character.Bag[p[3]-1].ID {
			return nil
		}
		_, err := s.redeemItemCard(ctx, c, p[3])
		return err
	case protocol.GiftCardDetails:
		if len(p) != 8 || p[4] != 1 {
			return protocol.ErrMalformed
		}
		// Legacy can submit from its Gec cache; reconstruct the owned source if no dialog is pending.
		if c.itemCard == nil {
			if p[3] < 1 || p[3] > game.BagSize {
				return protocol.ErrMalformed
			}
			c.itemCard = &pendingItemCard{p[3], c.character.Bag[p[3]-1]}
		}
		if c.itemCard.Slot != p[3] {
			return nil
		}
		return s.exchangeItemCard(ctx, c, p[5], binary.LittleEndian.Uint16(p[6:]))
	}
	return protocol.ErrMalformed
}
func (s *Server) exchangeItemCard(ctx context.Context, c *Session, index byte, rewardID uint16) error {
	pending := c.itemCard
	if pending == nil || c.event != nil || c.battle != nil || c.storm || c.beach != nil {
		return nil
	}
	card, ok := s.Assets.ItemCards[pending.Item.ID]
	if !ok || index == 0 || int(index) > len(card.Rewards) {
		return protocol.ErrMalformed
	}
	r := card.Rewards[index-1]
	if r.ItemID != rewardID {
		return protocol.ErrMalformed
	}
	if card.MaleItem != 0 {
		id := card.MaleItem
		if c.character.Body == game.BodySmallFemale || c.character.Body == game.BodyBigFemale {
			id = card.FemaleItem
		}
		if id != rewardID {
			return protocol.ErrMalformed
		}
	}
	limit, ok := s.stackLimit(r.ItemID)
	if !ok || r.Quantity == 0 {
		return protocol.ErrMalformed
	}
	var adds []game.Addition
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		current := next.Bag[pending.Slot-1]
		if current != pending.Item || current.Empty() || current.Locked {
			return game.ErrInvalidItem
		}
		if err := next.Bag.Remove(pending.Slot, 1); err != nil {
			return err
		}
		var err error
		adds, err = next.Bag.Grant(game.Item{ID: r.ItemID}, int(r.Quantity), limit, s.Assets.Items)
		return err
	}, *c.character)
	if errors.Is(err, game.ErrInventoryFull) || errors.Is(err, game.ErrInvalidItem) {
		c.itemCard = nil
		return s.sendAll(c, [][]byte{{protocol.CommandInventory, protocol.InventoryItemUse}, serverwire.HeadBanner("Cannot exchange this card. Check inventory space and ownership.")})
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	c.itemCard = nil
	return s.sendAll(c, [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, pending.Slot, 1}, next.Bag.AdditionPacket(adds), protocol.Builder{protocol.CommandInventory, protocol.InventoryGiftCard, protocol.GiftCardRedeemed}.U16(pending.Item.ID).U8(1).U16(r.ItemID).U8(r.Quantity), {protocol.CommandInventory, protocol.InventoryItemUse}})
}
