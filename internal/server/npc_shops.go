package server

import (
	"context"
	"errors"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

const (
	npcShopCartLimit                = 10
	npcShopMemberBytes              = 5
	npcShopWeaponsKind         byte = 2
	npcShopPropsKind           byte = 3
	npcShopFallbackPrice            = 10
	npcShopSalePriceMultiplier      = 10
	npcArmorMerchantTemplate        = 13005
	npcWeaponMerchantTemplate       = 13006
	npcPropsMerchantTemplate        = 13007
)

func (s *Server) openNPCShop(c *Session, service uint16) error {
	kind, catalog := byte(npcShopWeaponsKind), uint32(protocol.ShopCatalogWeapons)
	if service == world.ServicePropsShop {
		kind, catalog = npcShopPropsKind, protocol.ShopCatalogProps
	}
	if service == world.ServiceArmorShop {
		catalog = protocol.ShopCatalogArmor
	}
	c.view.ShopItems = map[uint16]uint16{}
	for _, group := range s.Assets.Maps[c.character.Map].Interactive {
		if len(group.Header) == 0 || group.Header[0] != kind || len(group.Members)%npcShopMemberBytes != 0 || len(group.Members) == 0 {
			continue
		}
		for i := 0; i < len(group.Members); i += npcShopMemberBytes {
			record := group.Members[i:]
			id := uint16(record[1]) | uint16(record[2])<<8
			if record[0] != 0 && id != 0 {
				c.view.ShopItems[uint16(record[0])] = id
			}
		}
		break
	}
	if len(c.view.ShopItems) == 0 {
		for i, id := range s.Assets.NPCShopDefaults[service] {
			c.view.ShopItems[uint16(i+1)] = id
		}
	}
	if c.event != nil && c.event.ev != nil && c.event.branch >= 0 && c.event.branch < len(c.event.ev.Branches) {
		cond := world.DecodeCond(c.event.ev.Branches[c.event.branch].Condition)
		if cond.Kind == world.ConditionChoiceResult && merchantSellChoice(cond.W2) {
			return s.openNPCSaleWindow(c)
		}
	}
	return c.send(protocol.Builder{protocol.CommandShopCatalog, protocol.ShopCatalogOpen}.U32(catalog))
}

func merchantSellChoice(answer uint16) bool { return answer == 31 || answer == 41 || answer == 2 }
func (s *Server) openNPCSaleWindow(c *Session) error {
	sub := byte(protocol.ShopEquipmentWindow)
	if c.saleMode == 1 {
		sub = protocol.ShopPropsWindow
	}
	return c.send([]byte{protocol.CommandShop, sub})
}

// Some native merchants have only a question and no callback branch. Restrict
// this fallback to verified vendor templates; unrelated questions cannot open shops.
func (s *Server) merchantChoice(c *Session, es *eventSession, answer uint16) (bool, error) {
	var service uint16
	for _, npc := range s.Assets.Maps[c.character.Map].NPCs {
		if npc.ClickID != es.click {
			continue
		}
		switch npc.Template {
		case npcArmorMerchantTemplate:
			service = world.ServiceArmorShop
		case npcWeaponMerchantTemplate:
			service = world.ServiceWeaponShop
		case npcPropsMerchantTemplate:
			service = world.ServicePropsShop
		}
		break
	}
	if service == 0 {
		return false, nil
	}
	buy := answer == 30 || answer == 40 || answer == 1
	if !buy && !merchantSellChoice(answer) {
		return false, nil
	}
	// Keep es available while opening so a sell choice sends only its sale form.
	if err := s.finishEvent(c, es); err != nil {
		return true, err
	}
	if merchantSellChoice(answer) {
		// Configure the catalog without sending a second buy window.
		c.saleMode = 0
		if service == world.ServicePropsShop {
			c.saleMode = 1
		}
		c.saleMap = c.character.Map
		return true, s.openNPCSaleWindow(c)
	}
	return true, s.openService(c, service)
}

type npcShopCartEntry struct {
	item  uint16
	count byte
}

func decodeNPCShopCart(p []byte) ([]npcShopCartEntry, error) {
	if len(p) == 0 {
		return nil, protocol.ErrMalformed
	}
	width, start := 0, 0
	switch {
	case len(p) >= 3 && (len(p)-1)%2 == 0 && int(p[0]) == (len(p)-1)/2:
		width, start = 2, 1
	case len(p) >= 4 && (len(p)-1)%3 == 0 && int(p[0]) == (len(p)-1)/3:
		width, start = 3, 1
	case len(p)%3 == 0:
		width = 3
	case len(p)%2 == 0:
		width = 2
	default:
		return nil, protocol.ErrMalformed
	}
	if (len(p)-start)/width > npcShopCartLimit {
		return nil, protocol.ErrMalformed
	}
	var entries []npcShopCartEntry
	for i := start; i < len(p); i += width {
		id, count := uint16(p[i]), p[i+1]
		if width == 3 {
			id = uint16(p[i]) | uint16(p[i+1])<<8
			count = p[i+2]
		}
		if id == 0 || count == 0 {
			return nil, protocol.ErrMalformed
		}
		entries = append(entries, npcShopCartEntry{id, count})
	}
	return entries, nil
}

func (s *Server) buyNPCItems(ctx context.Context, c *Session, p []byte) error {
	result := byte(protocol.ShopBuyRejected)
	reply := func() error { return c.send([]byte{protocol.CommandShop, protocol.ShopBuy, result}) }
	entries, err := decodeNPCShopCart(p[2:])
	if err != nil {
		return err
	}
	if c.view == nil || len(c.view.ShopItems) == 0 || c.saleMap != c.character.Map || c.event != nil {
		return reply()
	}
	var total uint64
	for i := range entries {
		e := &entries[i]
		id, found := c.view.ShopItems[e.item]
		if !found {
			for _, candidate := range c.view.ShopItems {
				if candidate == e.item {
					id, found = candidate, true
					break
				}
			}
		}
		if !found {
			return reply()
		}
		if _, known := s.Assets.Items[id]; !known {
			return reply()
		}
		e.item = id
		price := uint64(s.Assets.NPCBuyPrices[id])
		if price == 0 {
			price = uint64(s.Assets.SalePrices[id].Price) * npcShopSalePriceMultiplier
		}
		if price == 0 {
			price = npcShopFallbackPrice
		}
		total += price * uint64(e.count)
	}
	if total > game.MaxGold {
		result = protocol.ShopBuyNotEnoughGold
		return reply()
	}
	insufficient := errors.New("NPC purchase funds unavailable")
	var adds []game.Addition
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		if uint64(next.Gold) < total {
			return insufficient
		}
		for _, e := range entries {
			part, err := next.Bag.Grant(game.Item{ID: e.item}, int(e.count), s.Assets.Items[e.item].StackLimit(), s.Assets.Items)
			if err != nil {
				return err
			}
			adds = append(adds, part...)
		}
		next.Gold -= uint32(total)
		return nil
	}, *c.character)
	if errors.Is(err, insufficient) {
		result = protocol.ShopBuyNotEnoughGold
		return reply()
	}
	if errors.Is(err, game.ErrInventoryFull) {
		result = protocol.ShopBuyInventoryFull
		return reply()
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	return s.sendAll(c, [][]byte{next.Bag.AdditionPacket(adds), protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold), {protocol.CommandShop, protocol.ShopBuy, protocol.ShopBuySuccess}})
}
