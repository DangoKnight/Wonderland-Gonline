package server

import (
	"context"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

// NPC services opened by EVE action 7 (EveEventInterpreter case 7, Player.Open*).

// openService opens a service window. Caller holds worldMu.
func (s *Server) openService(c *Session, action uint16) error {
	switch action {
	case world.ServicePetHotel:
		return s.sendAll(c, append(s.hotelPackets(c.character), []byte{protocol.CommandNPCService, protocol.PetHotelOpen}))
	case world.ServiceWeaponShop, world.ServicePropsShop, world.ServiceArmorShop: // Weapon shop, props shop, armor shop: selling only, as in C#.
		mode := 0
		if action == world.ServicePropsShop {
			mode = 1
		}
		c.saleMode, c.saleMap = mode, c.character.Map
		if mode == 0 {
			return c.send([]byte{protocol.CommandShop, protocol.ShopEquipmentWindow})
		}
		return c.send([]byte{protocol.CommandShop, protocol.ShopPropsWindow})
	case world.ServicePropsKeeper, world.ServiceStockKeeper: // Props keeper and stock keeper both open item storage.
		return s.sendAll(c, [][]byte{{protocol.CommandStorage, protocol.StorageOpen}, c.character.Storage.Packet(protocol.CommandStorage, protocol.StorageItems), {protocol.CommandLegacyStorage, protocol.LegacyStorageWireCode6}, {protocol.CommandEvent, protocol.EventChoice}, {protocol.CommandCharacterSelection, protocol.CharacterSelectionRecordPoint, 0x98, 0x98, 1, 0, 0}, {protocol.CommandEvent, protocol.EventResume}})
	case world.ServiceClinic, world.ServiceClinicAlternate: // Clinic: offer a free rest when HP or SP is missing.
		full := c.character.Combat(s.Assets.Items)
		needs := int64(c.character.HP) < int64(full.MaxHP) || int64(c.character.SP) < int64(full.MaxSP)
		for _, pet := range c.character.Pets {
			full := pet.Combat(s.Assets.Items)
			needs = needs || pet.HP < full.MaxHP || pet.SP < full.MaxSP
		}
		offer := uint32(0xffffffff)
		c.restMap = 0
		if needs {
			c.restMap, offer = c.character.Map, 0
		}
		return c.send(protocol.Builder{protocol.CommandNPCService, protocol.ClinicRestOffer}.U32(offer))
	}
	return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
}

// confirmRest is Player.ConfirmNpcRest (AC31:1). Caller holds worldMu.
func (s *Server) confirmRest(ctx context.Context, c *Session) error {
	if c.restMap == 0 || c.restMap != c.character.Map || c.battle != nil {
		return nil
	}
	next := c.character.Clone()
	next.Refill(s.Assets.Items)
	for i := range next.Pets {
		next.Pets[i].Normalize(s.Assets.Items, true)
	}
	if e := s.commit(ctx, c, next); e != nil {
		return e
	}
	c.restMap = 0
	packets := next.StatPackets(s.Assets.Items)
	for _, pet := range next.Pets {
		packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items)...)
	}
	return s.sendAll(c, append(packets, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateRestEffect}.U32(next.ID), []byte{protocol.CommandNPCService, protocol.ClinicRestConfirm, 0}))
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
		next.Bag[slot-1] = game.Item{}
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, count})
	}
	if e := s.commit(ctx, c, next); e != nil {
		return 0, nil, e
	}
	return saleSuccess, append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold)), nil
}

// storageCommand is AC30: 1 withdraws and 2 deposits selected slots; with two slots,
// 3 moves within storage, 4 deposits into a storage slot and 5 withdraws into a bag
// slot. A fresh storage snapshot always follows. Caller holds worldMu.
func (s *Server) storageCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 || p[1] < protocol.StorageWithdraw || p[1] > protocol.StorageWithdrawToSlot {
		return nil
	}
	data := p[2:]
	next := c.character.Clone()
	var packets [][]byte
	changed := false
	move := func(fromBag, toBag bool, from, to byte) {
		src, dst := &next.Storage, &next.Storage
		if fromBag {
			src = &next.Bag
		}
		if toBag {
			dst = &next.Bag
		}
		if moved, adds := s.transfer(src, dst, from, to); moved > 0 {
			changed = true
			if fromBag {
				packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, from, moved})
			}
			if toBag {
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
		if e := s.commit(ctx, c, next); e != nil {
			return e
		}
	} else {
		packets = nil
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
