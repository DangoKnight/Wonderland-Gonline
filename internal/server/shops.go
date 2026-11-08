package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

// NPC services opened by EVE action 7 (EveEventInterpreter case 7, Player.Open*).

// openService opens a service window. Caller holds worldMu.
func (s *Server) openService(c *Session, action uint16) error {
	switch action {
	case world.ServiceMessageBoard:
		if c.event != nil {
			c.event.messageBoardOwner = c.character.ID
			return nil
		}
		return c.send([]byte{protocol.CommandMessageBoard, protocol.MessageBoardOpen})
	case world.ServiceRebornJobSelection:
		if c.event != nil {
			c.event.deferredServiceOwner, c.event.deferredServiceKind = c.character.ID, action
			return nil
		}
		if c.character.Reborn || c.character.Level < game.RebornMinimumLevel {
			return c.send(serverwire.HeadBanner("Rebirth requires level 100 and a character that has not reborn."))
		}
		c.view.RebornFormOpen = true
		return c.send([]byte{protocol.CommandRebirthJobSelection, protocol.RebirthJobConfirm})
	case world.ServiceCompanionRebirth:
		if c.event != nil {
			c.event.deferredServiceOwner, c.event.deferredServiceKind = c.character.ID, action
			return nil
		}
		c.view.CompanionRebirthOpen = true
		return c.send([]byte{protocol.CommandCompanionRebirth, protocol.CompanionRebirthOpen})
	case world.ServicePetHotel:
		s.holdNPCService(c, action)
		c.hotelMap = c.character.Map
		return s.publishNPCService(c, append(s.hotelPackets(c.character), []byte{protocol.CommandNPCService, protocol.PetHotelOpen})...)
	case world.ServiceWeaponShop, world.ServicePropsShop, world.ServiceArmorShop: // Weapon shop, props shop, armor shop: selling only, as in C#.
		mode := 0
		if action == world.ServicePropsShop {
			mode = 1
		}
		c.saleMode, c.saleMap = mode, c.character.Map
		return s.openNPCShop(c, action)
	case world.ServicePropsKeeper, world.ServiceStockKeeper: // Props keeper and stock keeper both open item storage.
		s.holdNPCService(c, action)
		c.storageMap = c.character.Map
		// FUN_001ded68 marks the dialogue ready to acknowledge when storage
		// closes. Retain its event until AC20:6; an early resume followed by an
		// ignored close ACK leaves Legacy waiting and unable to click or warp.
		return s.publishNPCService(c, []byte{protocol.CommandStorage, protocol.StorageOpen}, c.character.Storage.Packet(protocol.CommandStorage, protocol.StorageItems), []byte{protocol.CommandLegacyStorage, protocol.LegacyStorageWireCode6})
	case world.ServiceBank: // Bank counter: the dialogue close would hide the form, so open it after the resume.
		if c.event != nil {
			c.event.bankOwner = c.character.ID
			return nil
		}
		return s.openMoneyBank(c)
	case world.ServiceClinic: // Clinic: offer a free rest when HP or SP is missing.
		s.holdNPCService(c, action)
		full := c.character.Combat(s.Assets.Items)
		needs := int64(c.character.HP) < int64(full.MaxHP) || int64(c.character.SP) < int64(full.MaxSP)
		for _, pet := range c.character.Pets {
			full := pet.Combat(s.Assets.Items, s.petTemplate(pet.ID))
			needs = needs || pet.HP < full.MaxHP || pet.SP < full.MaxSP
		}
		offer := uint32(0xffffffff)
		c.restMap = 0
		if needs {
			c.restMap, offer = c.character.Map, 0
		}
		return s.publishNPCService(c, protocol.Builder{protocol.CommandNPCService, protocol.ClinicRestOffer}.U32(offer))
	}
	if c.event == nil {
		return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
	return nil // The owning event publishes its single completion/resume.
}

// confirmRest is Player.ConfirmNpcRest (AC31:1). Caller holds worldMu.
func (s *Server) confirmRest(ctx context.Context, c *Session) error {
	if c.restMap == 0 || c.restMap != c.character.Map || c.battle != nil {
		return nil
	}
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		if next.Map != c.character.Map {
			return errNPCResourceConflict
		}
		next.Refill(s.Assets.Items)
		for i := range next.Pets {
			next.Pets[i].Normalize(s.Assets.Items, true, s.petTemplate(next.Pets[i].ID))
		}
		return nil
	}, *c.character)
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	c.restMap = 0
	packets := next.StatPackets(s.Assets.Items)
	for _, pet := range next.Pets {
		packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items, s.petTemplate(pet.ID))...)
	}
	packets = append([][]byte{{protocol.CommandNPCService, protocol.ClinicRestAnimation}}, packets...)
	return s.sendAll(c, append(packets, []byte{protocol.CommandNPCService, protocol.ClinicRestConfirm, 0}))
}

// Sale results (NpcSaleResult).
const (
	saleSuccess     = 0
	saleGoldLimit   = 1
	saleUnavailable = 2
	saleTooMany     = 3
	saleRejected    = 4
)

// sellItems is AC27:2 (Inventory.SellToNpc): whole selected stacks, the last byte is
// the shop mode. Equipment sells only in mode 0 and other items only in mode 1.
func (s *Server) sellItems(ctx context.Context, c *Session, p []byte) error {
	if c.battle != nil {
		return nil
	}
	if len(p) >= 2 && p[1] == protocol.ShopBuy {
		return s.buyNPCItems(ctx, c, p)
	}
	if len(p) < 2 || p[1] != protocol.ShopSell {
		return nil
	}
	data := p[2:]
	result, packets := byte(saleRejected), [][]byte(nil)
	if len(data) >= 2 && data[len(data)-1] <= 1 {
		var e error
		result, packets, e = s.sell(ctx, c, data[:len(data)-1], int(data[len(data)-1]))
		if e != nil {
			return e
		}
	}
	return s.sendAll(c, append(packets, []byte{protocol.CommandShop, protocol.ShopSell, result}))
}

func (s *Server) sell(ctx context.Context, c *Session, slots []byte, mode int) (byte, [][]byte, error) {
	if len(slots) > game.BagSize {
		return saleTooMany, nil, nil
	}
	seen := map[byte]bool{}
	for _, slot := range slots {
		if slot < 1 || slot > game.BagSize || seen[slot] {
			return saleRejected, nil, nil
		}
		seen[slot] = true
	}
	if c.saleMode != mode || c.saleMap != c.character.Map {
		return saleRejected, nil, nil
	}
	next := c.character.Clone()
	total := uint64(0)
	for _, slot := range slots {
		item := next.Bag[slot-1]
		if item.Empty() {
			return saleUnavailable, nil, nil
		}
		price, ok := s.Assets.SalePrices[item.ID]
		equipment := s.Assets.Items[item.ID].EquipSlot >= 1 && s.Assets.Items[item.ID].EquipSlot <= 6
		if !ok || price.Flags&16 != 0 || (mode == 0) != equipment {
			return saleRejected, nil, nil
		}
		total += uint64(price.Price) * uint64(item.Count)
	}
	if total > game.MaxGold || uint64(next.Gold)+total > game.MaxGold {
		return saleGoldLimit, nil, nil
	}
	next.Gold += uint32(total)
	var packets [][]byte
	for _, slot := range slots {
		count := next.Bag[slot-1].Count
		price := s.Assets.SalePrices[next.Bag[slot-1].ID].Price * uint32(count)
		next.Bag[slot-1] = game.Item{}
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, count}, protocol.Builder{protocol.CommandGold, protocol.GoldGain}.U32(price))
	}
	if e := s.commitNPCResources(ctx, c, next); e != nil {
		return 0, nil, e
	}
	return saleSuccess, packets, nil
}

// storageCommand is AC30: 1 withdraws and 2 deposits selected slots; with two slots,
// 3 moves within storage, 4 deposits into a storage slot and 5 withdraws into a bag
// slot. A fresh storage snapshot always follows. Caller holds worldMu.
func (s *Server) storageCommand(ctx context.Context, c *Session, p []byte) error {
	if c.storageMap == 0 || c.storageMap != c.character.Map || c.battle != nil {
		return nil
	}
	if len(p) < 2 || p[1] < protocol.StorageWithdraw || p[1] > protocol.StorageWithdrawToSlot {
		return nil
	}
	data := p[2:]
	next := c.character.Clone()
	var packets [][]byte
	changed := false
	incremental := p[1] == protocol.StorageWithdraw || p[1] == protocol.StorageDeposit
	move := func(fromBag, toBag bool, from, to byte) {
		src, dst := &next.Storage, &next.Storage
		if fromBag {
			src = &next.Bag
		}
		if toBag {
			dst = &next.Bag
		}
		if from < 1 || from > game.BagSize {
			return
		}
		item := src[from-1]
		if moved, adds := s.transfer(src, dst, from, to); moved > 0 {
			changed = true
			if fromBag {
				packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, from, moved})
			}
			if incremental {
				item.Count = moved
				if fromBag {
					q := game.AcquisitionPacket(item)
					q[0], q[1] = protocol.CommandStorage, protocol.StorageAdd
					packets = append(packets, q)
				} else {
					packets = append(packets, []byte{protocol.CommandStorage, protocol.StorageRemove, from, moved}, game.AcquisitionPacket(item))
				}
			} else if toBag {
				packets = append(packets, next.Bag.AdditionPacket(adds))
			}
		}
	}
	switch sub := p[1]; {
	case (sub == protocol.StorageWithdraw || sub == protocol.StorageDeposit) && len(data) > 0 && len(data) <= game.BagSize:
		seen := map[byte]bool{}
		for _, slot := range data {
			if !seen[slot] {
				seen[slot] = true
				move(sub == protocol.StorageDeposit, sub == protocol.StorageWithdraw, slot, 0)
			}
		}
	case len(data) == 2 && data[0] >= 1 && data[0] <= game.BagSize && data[1] >= 1 && data[1] <= game.BagSize:
		switch sub {
		case protocol.StorageReorder:
			move(false, false, data[0], data[1])
		case protocol.StorageDepositToSlot:
			move(true, false, data[1], data[0])
		case protocol.StorageWithdrawToSlot:
			move(false, true, data[1], data[0])
		}
	}
	if changed {
		if e := s.commitNPCResources(ctx, c, next); e != nil {
			return e
		}
	} else {
		packets = nil
	}
	if incremental {
		unlock := byte(protocol.StorageWithdrawUnlock)
		if p[1] == protocol.StorageDeposit {
			unlock = protocol.StorageDepositUnlock
		}
		return s.sendAll(c, append(packets, []byte{protocol.CommandStorage, unlock}))
	}
	// Storage lists are additive: clear the client copy before the snapshot.
	return s.sendAll(c, append(packets, []byte{protocol.CommandStorage, protocol.StorageOpen}, c.character.Storage.Packet(protocol.CommandStorage, protocol.StorageItems), []byte{protocol.CommandStorage, protocol.StorageDepositComplete}, []byte{protocol.CommandStorage, protocol.StorageWithdrawComplete}))
}

// transfer is AC30.Transfer: an automatic move (to 0) stacks first then uses empty
// slots and must fit completely; a targeted move takes what fits. It returns the
// amount moved and the destination slots that received it.
func (s *Server) transfer(src, dst *game.Inventory, from, to byte) (byte, []game.Addition) {
	if from < 1 || from > game.BagSize || to > game.BagSize || (src == dst && from == to) {
		return 0, nil
	}
	item := src[from-1]
	if item.Empty() || item.Locked {
		return 0, nil
	}
	limit, known := s.stackLimit(item.ID)
	if !known {
		return 0, nil
	}
	if src == dst && to != 0 {
		moved, err := src.Move(from, to, item.Count, limit, s.Assets.Items)
		if err != nil {
			return 0, nil
		}
		return moved, []game.Addition{{Slot: to, Count: moved}}
	}
	source, target := *src, *dst
	if to == 0 {
		if err := source.Remove(from, item.Count); err != nil {
			return 0, nil
		}
		if src == dst {
			target = source
		}
		adds, err := target.Grant(item, int(item.Count), limit, s.Assets.Items)
		if err != nil {
			return 0, nil
		}
		if src != dst {
			*src = source
		}
		*dst = target
		return item.Count, adds
	}
	current := target[to-1]
	if current.Locked {
		return 0, nil
	}
	capacity := limit
	if !current.Empty() {
		if current.ID != item.ID || current.Damage != item.Damage || current.Metadata != item.Metadata {
			return 0, nil
		}
		capacity -= min(current.Count, limit)
	}
	moved := min(item.Count, capacity)
	if moved == 0 || source.Remove(from, moved) != nil {
		return 0, nil
	}
	if current.Empty() {
		if !target.CanPlace(to, item, s.Assets.Items) {
			return 0, nil
		}
		target[to-1] = item
		target[to-1].Count = moved
	} else {
		target[to-1].Count += moved
	}
	*src, *dst = source, target
	return moved, []game.Addition{{Slot: to, Count: moved}}
}

// Menu warp destinations (AC5.Recv17).
var (
	starterBeach = world.Destination{Map: 11016, X: 1181, Y: 243}
	carnie       = world.Destination{Map: 11094, X: 1180, Y: 875}
)

// menuCommand is AC5: 17 warps to the starter beach (1), the record point (2) or
// Carnie (3), remembering where Carnie's exit returns; 7 refreshes the sprite.
// Caller holds worldMu.
func (s *Server) menuCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.CharacterStateRefreshRequest:
		packets := [][]byte{protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(c.character.ID).U8(0)}
		if s.Assets.LuckyDraw.TotalWeight > 0 {
			packets = append(packets, s.luckyDrawCatalogPacket(c.character, time.Now()))
		}
		return s.sendAll(c, packets)
	case protocol.CharacterStateRefresh:
		if s.Assets.LuckyDraw.TotalWeight > 0 {
			return c.send(s.luckyDrawCatalogPacket(c.character, time.Now()))
		}
		return nil
	case protocol.CharacterStateTeleport:
		choice := byte(1)
		if len(p) > 2 {
			choice = p[2]
		}
		dst := starterBeach
		switch choice {
		case 3:
			return s.visitCarnie(ctx, c)
		case 2:
			if point := c.character.RecordPoint; point != nil && point.Map != 0 {
				dst = world.Destination{Map: point.Map, X: point.X, Y: point.Y}
			}
		}
		return s.commandTeleport(ctx, c, dst)
	}
	return nil
}

const (
	npcShopCartLimit                = 10
	npcShopMemberBytes              = 5
	npcShopWeaponsKind         byte = 2
	npcShopPropsKind           byte = 3
	npcShopItemKind            byte = 2
	npcShopOptionType          byte = 3
	npcShopFallbackPrice            = 10
	npcShopSalePriceMultiplier      = 10
	npcArmorMerchantTemplate        = 13005
	npcWeaponMerchantTemplate       = 13006
	npcPropsMerchantTemplate        = 13007
)

func (s *Server) openNPCShop(c *Session, service uint16) error {
	s.holdNPCService(c, service)
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
			if record[0] != 0 && id != 0 && record[3] == npcShopItemKind && record[4] == npcShopOptionType {
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
	return s.publishNPCService(c, protocol.Builder{protocol.CommandShopCatalog, protocol.ShopCatalogOpen}.U32(catalog))
}

func merchantSellChoice(answer uint16) bool { return answer == 31 || answer == 41 || answer == 2 }
func (s *Server) openNPCSaleWindow(c *Session) error {
	sub := byte(protocol.ShopEquipmentWindow)
	service := uint16(world.ServiceWeaponShop)
	if c.saleMode == 1 {
		sub = protocol.ShopPropsWindow
		service = world.ServicePropsShop
	}
	s.holdNPCService(c, service)
	return s.publishNPCService(c, []byte{protocol.CommandShop, sub})
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
	// The window replaces the question but retains its close-ACK token.
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
	if c.view == nil || len(c.view.ShopItems) == 0 || c.saleMap != c.character.Map || c.event != nil && !c.event.shopCart && !ownsNPCShop(c) {
		return reply()
	}
	var total uint64
	var costs []uint32
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
		if price*uint64(e.count) > game.MaxGold {
			result = protocol.ShopBuyNotEnoughGold
			return reply()
		}
		costs = append(costs, uint32(price*uint64(e.count)))
	}
	if total > game.MaxGold {
		result = protocol.ShopBuyNotEnoughGold
		return reply()
	}
	insufficient := errors.New("NPC purchase funds unavailable")
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		if uint64(next.Gold) < total {
			return insufficient
		}
		for _, e := range entries {
			_, err := next.Bag.Grant(game.Item{ID: e.item}, int(e.count), s.Assets.Items[e.item].StackLimit(), s.Assets.Items)
			if err != nil {
				return err
			}
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
	if c.event != nil {
		c.event.shopCart = false
	}
	var packets [][]byte
	for i, e := range entries {
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldSpend}.U32(costs[i]), game.AcquisitionPacket(game.Item{ID: e.item, Count: e.count}))
	}
	return s.sendAll(c, append(packets, []byte{protocol.CommandShop, protocol.ShopBuy, protocol.ShopBuySuccess}))
}

// QuestNpc.ResolveTalkIdForNpc's default native Kelan welcome line.
const fallbackNPCGreetingTalk = 0x21284c
const fallbackDialogueStep = 1
const fallbackNPCPortrait = 3

func (s *Server) npcGreeting(c *Session, n world.NPC) error {
	// Authored events, visibility, services and resource nodes have already run.
	es := &eventSession{mapID: c.character.Map, click: n.ClickID}
	c.event = es
	es.onComplete = func() error {
		if c.event != es || c.character.Map != es.mapID {
			return nil
		}
		return s.finishEvent(c, es)
	}
	text := s.Assets.Talks.ByID[fallbackNPCGreetingTalk]
	if text == "" {
		text = "Hello!"
	}
	return s.sendAll(c, [][]byte{
		{protocol.CommandMovement, protocol.MovementMovementLock, 1},
		serverwire.DialogueFrame(fallbackDialogueStep, fallbackNPCPortrait, byte(n.ClickID), fallbackNPCGreetingTalk),
		serverwire.SystemLine(fmt.Sprintf(" %s: %s", n.Name, text)),
	})
}

// serviceKind follows QuestNpc.Interact's priority and name/template heuristics.
func serviceKind(n world.NPC) byte {
	name := strings.ToLower(n.Name)
	contains := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(name, part) {
				return true
			}
		}
		return false
	}
	switch {
	case contains("props keep", "storage") || n.Template == 14134:
		return 4
	case contains("stock keep", "bank", "vault", "exchanger") || n.Template == 14181 || n.Template == 14157:
		return world.ServiceBank
	case contains("doctor", "witch", "clinic") || n.Template == 14151:
		return world.ServiceClinic
	case contains("pet hotel", "pet keep", "hotel") || n.Template == 14182 || n.Template == 14152:
		return 5
	case contains("props shop", "weapon shop", "armor shop") || n.Template == 13007 || n.Template == 13006 || n.Template == 13005:
		if n.Template == 13006 || contains("weapon") {
			return 1
		}
		return 2 // Legacy armor fallback uses the props-sale catalog too.
	}
	return 0
}

// fallbackService only runs when EVE has no event for this actor. Its callbacks
// use the same session token and cancellation path as native dialogue.
func (s *Server) fallbackService(ctx context.Context, c *Session, n world.NPC) (bool, error) {
	kind := serviceKind(n)
	if kind == 0 {
		return false, nil
	}
	if kind == world.ServicePropsKeeper || kind == world.ServiceStockKeeper || kind == world.ServiceBank || kind == world.ServicePetHotel {
		return true, s.openService(c, uint16(kind))
	}
	// ServiceQuestion uses native mode zero, so every answered question also
	// has a trailing AC20:6 distinct from the eventual service close ACK.
	es := &eventSession{mapID: c.character.Map, click: n.ClickID, choiceAcknowledged: true}
	c.event = es
	if err := c.send([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1}); err != nil {
		return true, err
	}
	if kind == world.ServiceClinic {
		es.onChoice = func(choice byte) error {
			if c.event != es || c.character.Map != es.mapID {
				return nil
			}
			switch choice {
			case 30, 1:
				return s.openService(c, world.ServiceClinic)
			case 31, 2:
				next := c.character.Clone()
				next.RecordPoint = &game.Location{Map: next.Map, X: next.X, Y: next.Y}
				if err := s.commit(ctx, c, next); err != nil {
					return err
				}
				return s.sendAll(c, [][]byte{serverwire.DialogueFrame(1, 3, byte(n.ClickID), 0x0379b6), serverwire.RecordPointStatus(next), {protocol.CommandEvent, protocol.EventStepComplete}})
			case 32:
				return s.openService(c, world.ServicePetHotel)
			}
			return s.cancelInteraction(c)
		}
		return true, c.send(serverwire.ServiceQuestion(n.ClickID, 3, 1))
	}
	initial, second := byte(5), byte(6)
	if kind == 1 {
		initial, second = 9, 8
	}
	es.onChoice = func(choice byte) error {
		if c.event != es || c.character.Map != es.mapID {
			return nil
		}
		if choice != 30 && choice != 1 {
			if !merchantSellChoice(uint16(choice)) {
				return s.cancelInteraction(c)
			}
			c.saleMode, c.saleMap = 1, c.character.Map
			if kind == world.ServiceWeaponShop {
				c.saleMode = 0
			}
			c.view.ShopItems = nil
			return s.openNPCSaleWindow(c)
		}
		es.onChoice = func(choice byte) error {
			if c.event != es || c.character.Map != es.mapID {
				return nil
			}
			service := uint16(kind)
			if n.Template == npcArmorMerchantTemplate || strings.Contains(strings.ToLower(n.Name), "armor") {
				service = world.ServiceArmorShop
			}
			if choice == 30 || choice == 1 {
				return s.openService(c, service)
			}
			if merchantSellChoice(uint16(choice)) {
				c.saleMode, c.saleMap = 0, c.character.Map
				if service == world.ServicePropsShop {
					c.saleMode = 1
				}
				c.view.ShopItems = nil
				return s.openNPCSaleWindow(c)
			}
			return s.cancelInteraction(c)
		}
		return c.send(serverwire.ServiceQuestion(n.ClickID, second, 2))
	}
	return true, c.send(serverwire.ServiceQuestion(n.ClickID, initial, 1))
}

// bankCommand implements AC45:8/9/10 and the read-only default balance reply
// under worldMu. The registry owns loading, battle/minigame and trade gates;
// scripted interactions retain their ownership.
func (s *Server) bankCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.BankBalance:
		if len(p) != protocol.BankBalanceRequestBytes {
			return protocol.ErrMalformed
		}
	case protocol.BankDeposit, protocol.BankWithdraw:
		if len(p) != protocol.BankAmountRequestBytes {
			return protocol.ErrMalformed
		}
	case protocol.BankSetPIN, protocol.BankTransfer:
		// These source handlers only acknowledge success; there is no implemented
		// authentication or transfer to port. Never claim those operations succeeded.
	default:
		// AC45 defaults unknown subcommands to a balance query. Ignore their
		// operands; this path cannot perform a transfer or claim success.
		p = []byte{protocol.CommandBank, protocol.BankBalance}
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	if p[1] == protocol.BankSetPIN || p[1] == protocol.BankTransfer {
		return c.send(serverwire.HeadBanner("Bank PIN changes and character transfers are unavailable."))
	}
	if p[1] == protocol.BankBalance {
		return c.send(serverwire.BankBalancePacket(c.character, protocol.BankBalance))
	}
	amount := protocol.NewReader(p[2:]).U32()
	changed, err := s.transferBankGold(ctx, c, amount, p[1] == protocol.BankDeposit)
	if err != nil {
		return err
	}
	if !changed {
		return s.sendAll(c, [][]byte{serverwire.HeadBanner("The requested bank transaction is unavailable."), serverwire.BankBalancePacket(c.character, protocol.BankBalance)})
	}
	return s.sendAll(c, [][]byte{protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(c.character.Gold), serverwire.BankBalancePacket(c.character, p[1])})
}

// Both native counter and ATM paths use the same authoritative SQL transfer.
// Only the two committed currency fields enter the session/baseline; pending
// walking, resting and inventory state cannot be checkpointed by a bank request.
func (s *Server) transferBankGold(ctx context.Context, c *Session, amount uint32, deposit bool) (bool, error) {
	balances, changed, err := s.Store.TransferBankGold(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, amount, deposit)
	if err != nil {
		return false, err
	}
	oldGold := c.character.Gold
	c.character.Gold, c.character.BankGold = balances.Gold, balances.BankGold
	if c.autosaveBaseline != nil {
		c.autosaveBaseline.Gold, c.autosaveBaseline.BankGold = balances.Gold, balances.BankGold
	}
	if oldGold != balances.Gold {
		s.closeStall(c)
	}
	return changed, nil
}

// openMoneyBank shows the native money window and publishes the account balance.
func (s *Server) openMoneyBank(c *Session) error {
	return s.sendAll(c, [][]byte{
		{protocol.CommandLegacyStorage, protocol.MoneyBankOpen},
		protocol.Builder{protocol.CommandLegacyStorage, protocol.MoneyBankBankBalance}.U32(c.character.BankGold),
	})
}

// moneyBankCommand handles the bank counter's AC29 deposit and withdraw. The
// client's bank form clears its in-flight flag only when the matching
// completion arrives, so an accepted operation publishes the wallet, the
// account balance and that completion together.
func (s *Server) moneyBankCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	if p[1] != protocol.MoneyBankWithdraw && p[1] != protocol.MoneyBankDeposit {
		return nil
	}
	if len(p) != protocol.MoneyBankAmountRequestLen {
		return protocol.ErrMalformed
	}
	amount := protocol.NewReader(p[2:]).U32()
	changed, err := s.transferBankGold(ctx, c, amount, p[1] == protocol.MoneyBankDeposit)
	if err != nil {
		return err
	}
	var done byte
	if p[1] == protocol.MoneyBankDeposit {
		done = protocol.MoneyBankDepositComplete
	} else {
		done = protocol.MoneyBankWithdrawComplete
	}
	if !changed {
		// The native form keeps its in-flight flag until a completion arrives, so
		// reject with the native message and unlock the same way a success does.
		return s.sendAll(c, [][]byte{
			{protocol.CommandLegacyStorage, protocol.MoneyBankRejected},
			protocol.Builder{protocol.CommandLegacyStorage, protocol.MoneyBankBankBalance}.U32(c.character.BankGold),
			{protocol.CommandLegacyStorage, done},
		})
	}
	return s.sendAll(c, [][]byte{
		protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(c.character.Gold),
		protocol.Builder{protocol.CommandLegacyStorage, protocol.MoneyBankBankBalance}.U32(c.character.BankGold),
		{protocol.CommandLegacyStorage, done},
	})
}

// decodeHotel validates the entire AC31 selection before any mutation.
func decodeHotel(p []byte) (deposits, withdrawals []byte, ok bool) {
	if len(p) < 2 {
		return
	}
	d := p[2:]
	switch p[1] {
	case protocol.PetHotelWithdraw:
		withdrawals = d
	case protocol.PetHotelDeposit:
		deposits = d
	case protocol.PetHotelTransfer:
		if len(d) < 2 || d[0] > game.MaxPets || len(d) < int(d[0])+2 {
			return
		}
		n := int(d[0])
		m := int(d[n+1])
		if m > game.MaxHotelPets || len(d) != n+m+2 {
			return
		}
		deposits, withdrawals = d[1:n+1], d[n+2:]
	default:
		return
	}
	valid := func(slots []byte, limit byte) bool {
		seen := map[byte]bool{}
		for _, slot := range slots {
			if slot < 1 || slot > limit || seen[slot] {
				return false
			}
			seen[slot] = true
		}
		return len(slots) <= int(limit)
	}
	ok = len(deposits)+len(withdrawals) > 0 && valid(deposits, game.MaxPets) && valid(withdrawals, game.MaxHotelPets)
	return
}

// hotelPackets is Player.SendPetHotelList: explicit removals precede additive records.
func (s *Server) hotelPackets(char *game.Character) [][]byte {
	pets := slices.Clone(char.HotelPets)
	slices.SortFunc(pets, func(a, b game.Pet) int { return int(a.Slot) - int(b.Slot) })
	var packets [][]byte
	for slot := byte(1); slot <= game.MaxHotelPets; slot++ {
		if !slices.ContainsFunc(pets, func(p game.Pet) bool { return p.Slot == slot }) {
			packets = append(packets, []byte{protocol.CommandNPCService, protocol.NPCServiceWireCode4, slot})
		}
	}
	b := protocol.Builder{protocol.CommandNPCService, protocol.NPCServiceWireCode6}
	for _, pet := range pets {
		b = pet.HotelRecord(b, s.petTemplate(pet.ID))
	}
	if len(pets) > 0 {
		packets = append(packets, b)
	}
	return packets
}

// hotelCommand ports AC31's atomic exchange. Caller holds worldMu; client-slot
// registration and success packets are changed only after the durable commit.
func (s *Server) hotelCommand(ctx context.Context, c *Session, p []byte) error {
	deposits, withdrawals, ok := decodeHotel(p)
	if !ok || c.battle != nil || c.hotelMap == 0 || c.hotelMap != c.character.Map {
		return nil
	}
	unlock := func() error {
		return s.sendAll(c, [][]byte{{protocol.CommandNPCService, protocol.PetHotelStorageUnlock}, {protocol.CommandNPCService, protocol.PetHotelModalUnlock}})
	}
	next := c.character.Clone()
	var outgoing, incoming []game.Pet
	for _, slot := range deposits {
		i := slices.IndexFunc(next.Pets, func(p game.Pet) bool { return c.pets.slot(p.ID) == slot })
		if i < 0 || next.Pets[i].ID == 0 {
			return unlock()
		}
		outgoing = append(outgoing, next.Pets[i])
		next.Pets = slices.Delete(next.Pets, i, i+1)
	}
	for _, slot := range withdrawals {
		i := slices.IndexFunc(next.HotelPets, func(p game.Pet) bool { return p.Slot == slot })
		if i < 0 || next.HotelPets[i].ID == 0 {
			return unlock()
		}
		incoming = append(incoming, next.HotelPets[i])
		next.HotelPets = slices.Delete(next.HotelPets, i, i+1)
	}
	if len(next.Pets)+len(incoming) > game.MaxPets || len(next.HotelPets)+len(outgoing) > game.MaxHotelPets {
		return unlock()
	}
	team := append(slices.Clone(next.Pets), incoming...)
	for i, pet := range team {
		for _, other := range team[:i] {
			if game.SamePet(pet.ID, other.ID) {
				return unlock()
			}
		}
	}
	free := func(pets []game.Pet) byte {
		for slot := byte(1); ; slot++ {
			if !slices.ContainsFunc(pets, func(p game.Pet) bool { return p.Slot == slot }) {
				return slot
			}
		}
	}
	var packets [][]byte
	activeRemoved := false
	mountRemoved := false
	for _, pet := range outgoing {
		if next.ActiveMount != 0 && game.SamePet(next.ActiveMount, pet.ID) {
			mountRemoved = true
			next.ActiveMount = 0
		}
		if pet.Battle || game.SamePet(next.ActivePet, pet.ID) {
			activeRemoved = true
			next.ActivePet = 0
		}
		pet.Slot, pet.Battle = free(next.HotelPets), false
		packets = append(packets, []byte{protocol.CommandNPCService, protocol.NPCServiceWireCode3, pet.Slot, c.pets.slot(pet.ID)}, protocol.Builder{protocol.CommandPetControl, protocol.PetControlPetSlot}.U32(next.ID).U8(c.pets.slot(pet.ID)))
		next.HotelPets = append(next.HotelPets, pet)
	}
	for _, pet := range incoming {
		pet.Slot, pet.Battle = free(next.Pets), false
		next.Pets = append(next.Pets, pet)
	}
	if err := s.commitNPCResources(ctx, c, next); err != nil {
		return err
	}
	for _, pet := range outgoing {
		c.pets.release(pet.ID)
	}
	for i, pet := range incoming {
		c.pets.register(pet.ID)
		packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items, s.petTemplate(pet.ID))...)
		packets = append(packets, []byte{protocol.CommandNPCService, protocol.PetHotelCopyToTeam, withdrawals[i], c.pets.slot(pet.ID)}, []byte{protocol.CommandNPCService, protocol.NPCServiceWireCode4, withdrawals[i]})
	}
	if mountRemoved {
		packets = append(serverwire.UnmountPackets(next.ID), packets...)
		for _, packet := range serverwire.UnmountPackets(next.ID) {
			s.broadcastWorld(c, packet)
		}
	}
	if activeRemoved {
		packets = append([][]byte{{protocol.CommandBattlePet, protocol.BattlePetRest}}, packets...)
		s.broadcastWorld(c, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(next.ID))
	}
	if roster := s.petListPacket(c.character, c.pets); roster != nil {
		packets = append(packets, roster)
	}

	packets = append(packets, s.hotelPackets(c.character)...)
	responseUnlock := byte(protocol.PetHotelStorageUnlock)
	if len(outgoing) > 0 && len(incoming) == 0 {
		responseUnlock = protocol.PetHotelDepositUnlock
	}
	packets = append(packets, []byte{protocol.CommandNPCService, responseUnlock}, []byte{protocol.CommandNPCService, protocol.PetHotelModalUnlock})
	return s.sendAll(c, packets)
}
