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
		s.sendOrClose(c, petNamePacket(next.ID, c.pets.slot(pet.ID), s.companionName(pet.ID, pet.Name)))
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
