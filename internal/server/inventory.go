package server

import (
	"context"
	"errors"
	"fmt"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

// itemCommand handles bag and ground actions; the caller holds worldMu. Reference: AC23.Recv2/3/10 and
// GameMap.onItemPickup/onItemDrop, Player.WearEQ/unWearEQ. Remaining item operations are pending.
// Older handlers follow the C# reader and ignore trailing bytes. Compound and
// mall and pack-opening handlers require complete requests.
func (s *Server) itemCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.InventoryGiftCard:
		return s.itemCardCommand(ctx, c, p)
	case protocol.InventoryPetStallPurchase, protocol.InventoryVehicleStallPurchase:
		return s.bazaarTypedPurchase(ctx, c, p)
	case protocol.InventoryStallOpen, protocol.InventoryStallClose, protocol.InventoryStallBrowse, protocol.InventoryStallPurchase:
		return s.nativeStallCommand(ctx, c, p)
	case protocol.InventoryBambooDragonfly:
		if len(p) != 5 {
			return protocol.ErrMalformed
		}
		return s.bambooDragonfly(ctx, c, p[2], p[3])
	case protocol.InventoryStallListRequest:
		// AC23.Recv77 supplies only the empty global-list layout. Keep both
		// native loading replies; actual player shops use AC56 map signs/view.
		return s.sendAll(c, [][]byte{{protocol.CommandInventory, protocol.InventoryStallList, 0}, {protocol.CommandInventory, protocol.InventoryStallListComplete}})
	case protocol.InventoryOpenPack, protocol.InventoryOpenPackAlternate:
		return s.openPackCommand(ctx, c, p)
	case protocol.InventoryPotentialPill:
		return s.potentialPillCommand(ctx, c, p)
	case protocol.InventoryCompound, protocol.InventoryCompoundJunior, protocol.InventoryCompoundSuperior:
		return s.compoundCommand(ctx, c, p)
	case protocol.InventoryFishingStart:
		slot, err := fishingStartSlot(p)
		if err != nil {
			return err
		}
		return s.startFishing(ctx, c, slot)
	case protocol.InventoryMallBalance, protocol.InventoryMallCatalog, protocol.InventoryMallBuy:
		if p[1] == protocol.InventoryFishingStop {
			c.gathering = nil
			if err := fishingStopRequest(p); err != nil {
				return err
			}
			if c.fishing != nil {
				s.stopFishing(c)
				return nil
			}
			// Native stop and legacy mall balance share this subcommand.
			p = p[:2]
		}
		return s.legacyMallCommand(ctx, c, p)
	case protocol.InventoryPickup:
		if len(p) < 3 {
			return protocol.ErrMalformed
		}
		return s.pickup(ctx, c, p[2])
	case protocol.InventoryDrop:
		if len(p) < 5 {
			return protocol.ErrMalformed
		}
		return s.drop(ctx, c, p[2], p[3])
	case protocol.InventoryMove:
		if len(p) < 5 {
			return protocol.ErrMalformed
		}
		return s.moveItem(ctx, c, p[2], p[3], p[4])
	case protocol.InventoryEquip:
		if len(p) < 3 {
			return protocol.ErrMalformed
		}
		return s.changeEquipment(ctx, c, p[2], 0, true)
	case protocol.InventoryUnequip:
		if len(p) < 4 {
			return protocol.ErrMalformed
		}
		return s.changeEquipment(ctx, c, p[2], p[3], false)
	case protocol.InventoryPetEquip, protocol.InventoryPetUnequip:
		if (p[1] == protocol.InventoryPetEquip && len(p) != 4) || (p[1] == protocol.InventoryPetUnequip && len(p) != 5) {
			return nil
		}
		to := byte(0)
		if p[1] == protocol.InventoryPetUnequip {
			to = p[4]
		}
		return s.changePetEquipment(ctx, c, p[2], p[3], to, p[1] == protocol.InventoryPetEquip)
	case protocol.InventoryUse:
		// AC23.Recv96 accepts exactly one slot byte.
		if len(p) != 3 {
			return nil
		}
		return s.useItem(ctx, c, p[2])
	case protocol.InventoryItemUse:
		switch len(p) {
		case 3:
			return s.useItem(ctx, c, p[2])
		case 4:
			return s.useItemOn(ctx, c, p[2], p[3], 0)
		case 6:
			return s.useItemOn(ctx, c, p[2], p[3], uint16(p[4])|uint16(p[5])<<8)
		default:
			return protocol.ErrMalformed
		}
	case protocol.InventoryDestroy:
		if len(p) < 5 {
			return protocol.ErrMalformed
		}
		return s.destroyItem(ctx, c, p[2], p[3])
	}
	return ErrUnsupported
}

func (s *Server) stackLimit(id uint16) (byte, bool) {
	def, ok := s.Assets.Items[id]
	return def.StackLimit(), ok
}

// saveBag commits the bag before the session copy changes or any success packet is sent.
func (s *Server) saveBag(ctx context.Context, c *Session, bag game.Inventory) error {
	next := c.character.Clone()
	next.Bag = bag
	return s.commit(ctx, c, next)
}

func (s *Server) pickup(ctx context.Context, c *Session, slot byte) error {
	if slot == 0 {
		return nil
	}
	char := c.character
	target, ok := s.World.GroundAt(char.Map, slot)
	if !ok || !target.InRange(char.X, char.Y) {
		return nil
	}
	bag := char.Bag
	limit, known := s.stackLimit(target.Item.ID)
	var e error
	if known {
		_, e = bag.Grant(target.Item, 1, limit, s.Assets.Items)
	}
	// Unknown IDs cannot enter the bag; C# reports them as a full inventory too.
	if !known || e != nil {
		return c.send(serverwire.HeadBanner("Your inventory is full."))
	}
	now := time.Now()
	accountScope := s.World.AccountGroundScope(char.Map)
	personal := target.Native() && accountScope != 0
	if personal {
		grant := target.Item
		grant.Count = 1
		next, err := s.Store.ClaimAccountGroundItem(ctx, store.CharacterRef{Account: c.account.ID, ID: char.ID}, char.Map, accountScope, target.Slot, grant, limit, now, s.Assets.Items, char.Clone())
		if errors.Is(err, store.ErrResourceUnavailable) {
			return c.send(protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(uint16(target.Slot)))
		}
		if err != nil {
			return err
		}
		s.adoptSavedCharacter(c, next)
	} else if cooldown := target.RespawnDelay(); cooldown > 0 {
		cooldown = s.Assets.ResourceCooldown(store.ResourceGroundItem, char.Map, uint16(target.Slot), cooldown)
		grant := target.Item
		grant.Count = 1
		next, err := s.Store.ClaimGroundItem(ctx, store.CharacterRef{Account: c.account.ID, ID: char.ID}, store.ResourceKey{Kind: store.ResourceGroundItem, MapID: char.Map, NodeID: uint16(target.Slot)}, grant, limit, now, cooldown, s.Assets.Items, char.Clone())
		if errors.Is(err, store.ErrResourceUnavailable) {
			return nil
		}
		if err != nil {
			return err
		}
		s.adoptSavedCharacter(c, next)
	} else if e = s.saveBag(ctx, c, bag); e != nil {
		return e
	}
	s.World.Take(char.Map, target, now)
	if target.RespawnDelay() > 0 {
		expiry := now.Add(s.Assets.ResourceCooldown(store.ResourceGroundItem, char.Map, uint16(target.Slot), target.RespawnDelay()))
		s.World.RestoreGroundCooldowns(char.Map, map[uint16]time.Time{uint16(target.Slot): expiry})
	}
	if e = c.send(protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(uint16(target.Slot)).U8(1)); e != nil {
		return e
	}
	grant := target.Item
	grant.Count = 1
	if e = c.send(game.AcquisitionPacket(grant)); e != nil {
		return e
	}
	remove := protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(uint16(target.Slot))
	if personal {
		for _, peer := range s.world {
			if peer != c && peer.character != nil && peer.account.ID == c.account.ID && peer.tentOwner == 0 && s.World.AccountGroundScope(peer.character.Map) == accountScope {
				_ = peer.send(remove)
			}
		}
	} else {
		flight := (protocol.ObservedGroundPickup{Slot: uint16(target.Slot), Collector: char.ID, X: char.X, Y: char.Y}).Packet()
		// Resource visibility is independent of private player appearances.
		for _, peer := range s.world {
			if peer != c && sameScene(c, peer) {
				packet := []byte(remove)
				if peer.connectionNegotiated && peer.connectionFeatures&protocol.ConnectionObservedPickups != 0 && !c.invisible && s.samePlayerScene(c, peer) {
					packet = flight
				}
				_ = peer.send(packet)
			}
		}
	}
	return nil
}

func (s *Server) drop(ctx context.Context, c *Session, slot, count byte) error {
	if slot < 1 || slot > game.BagSize {
		return nil
	}
	char := c.character
	item := char.Bag[slot-1]
	if item.Empty() {
		return nil
	}
	if !s.Assets.Items[item.ID].Droppable() {
		// The client asks whether to destroy the item instead.
		return c.send(protocol.Builder{protocol.CommandInventory, protocol.InventoryWireCode212, protocol.InventoryDestroyPrompt, slot}.U16(item.ID).U8(count))
	}
	if count == 0 || count > item.Count {
		return nil
	}
	slots, ok := s.World.FreeGroundSlots(char.Map, int(count))
	if !ok {
		return c.send(serverwire.HeadBanner("There is no room to drop these items here."))
	}
	bag := char.Bag
	if e := bag.Remove(slot, count); e != nil {
		return nil
	}
	if e := s.saveBag(ctx, c, bag); e != nil {
		return e
	}
	s.World.Drop(char.Map, slots, item, char.X, char.Y)
	packet := protocol.Builder{protocol.CommandInventory, protocol.InventoryDrop}.U16(item.ID).U16(char.X).U16(char.Y).U32(0).Bytes(slots)
	if e := c.send(packet); e != nil {
		return e
	}
	if e := c.send([]byte{protocol.CommandInventory, protocol.InventoryRemove, slot, count}); e != nil {
		return e
	}
	s.broadcastWorld(c, packet)
	return nil
}

func (s *Server) moveItem(ctx context.Context, c *Session, from, count, to byte) error {
	if from < 1 || from > game.BagSize || to < 1 || to > game.BagSize || count < 1 || count > game.MaxItemStack {
		return nil
	}
	if c.character.ActiveVehicle != 0 && (from == c.character.VehicleSlot || to == c.character.VehicleSlot) {
		return nil
	}
	bag := c.character.Bag
	limit, known := s.stackLimit(bag[from-1].ID)
	if !known {
		return nil
	}
	moved, e := bag.Move(from, to, count, limit, s.Assets.Items)
	if e != nil {
		return nil
	}
	if e = s.saveBag(ctx, c, bag); e != nil {
		return e
	}
	return c.send([]byte{protocol.CommandInventory, protocol.InventoryMove, from, moved, to})
}

// broadcastMap sends to every published character on a map. Caller holds worldMu.
func (s *Server) broadcastMap(mapID uint16, packet []byte) {
	for _, peer := range s.world {
		if peer.character.Map == mapID && peer.tentOwner == 0 {
			if err := peer.send(packet); err != nil {
				peer.conn.Close()
			}
		}
	}
}

func (s *Server) respawnGround(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	for mapID, packets := range s.World.Respawn(now) {
		for _, packet := range packets {
			s.broadcastMap(mapID, packet)
		}
	}
}

// runRespawns ticks like GameMap.Process for ground items.
func (s *Server) runRespawns(ctx context.Context) {
	t := time.NewTicker(actorSimulationInterval)
	defer t.Stop()
	var elapsed time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.expirePartyLeadership(now)
			elapsed += actorSimulationInterval
			if elapsed < worldMaintenanceInterval {
				s.simulateWorldTick(now, false)
				continue
			}
			elapsed = 0
			s.respawnGround(now)
			s.respawnChests(now)
			s.respawnMapProps(ctx, now)
			s.reviveMonsters(now)
			s.simulateWorld(now)
			s.restCharacters(now)
			s.maintainGathering(ctx, now)
		}
	}
}

// changeEquipment handles AC23:11 (wear bag slot from) and AC23:12 (remove worn slot from
// to bag slot to). The original C# server did not broadcast the new appearance to
// map peers, so a nearby client only saw it after a map change; publish the
// equipment snapshot and sprite refresh here instead.
func (s *Server) changeEquipment(ctx context.Context, c *Session, from, to byte, wear bool) error {
	next := *c.character
	before := next.Combat(s.Assets.Items)
	var e error
	reply := []byte{protocol.CommandInventory, protocol.InventoryEquipmentChanged, from, to}
	if wear {
		e = next.Wear(from, s.Assets.Items)
		reply = []byte{protocol.CommandInventory, protocol.InventoryPetEquip, from, from}
	} else {
		e = next.Unwear(from, to, s.Assets.Items)
	}
	if e != nil {
		if errors.Is(e, game.ErrEquipLevel) {
			return c.send(serverwire.HeadBanner(game.ErrEquipLevel.Error()))
		}
		return nil
	}
	if e = s.Store.UpdateCharacter(ctx, c.account.ID, next.ID, func(char *game.Character) error {
		char.Bag, char.Equipment = next.Bag, next.Equipment
		return nil
	}); e != nil {
		return e
	}
	c.character.Bag, c.character.Equipment = next.Bag, next.Equipment
	s.broadcastWorld(c, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateEquipmentSnapshot}.U32(next.ID).Bytes(next.WornEquipment()))
	s.broadcastWorld(c, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U8(0))
	packets := append([][]byte{reply}, next.StatPackets(s.Assets.Items)...)
	if text := game.ChangeBanner("Equipment: ", before, next.Combat(s.Assets.Items)); text != "" {
		packets = append(packets, serverwire.HeadBanner(text))
	}
	for _, packet := range packets {
		if e = c.send(packet); e != nil {
			return e
		}
	}
	return nil
}

// allocateStats is AC08 for the character or a client pet slot: spend stat points,
// keep current HP/SP, and resend the stat block. The caller holds worldMu.
func (s *Server) allocateStats(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	target, requests := game.ParseStatAllocation(p[1], p[2:])
	if len(requests) == 0 {
		return nil
	}
	if target != 0 {
		i := s.clientPet(c, target)
		if i < 0 {
			return nil
		}
		next := c.character.Clone()
		if !next.Pets[i].Allocate(requests) {
			return nil
		}
		next.Pets[i].Normalize(s.Assets.Items, false, s.petTemplate(next.Pets[i].ID))
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		return s.sendAll(c, next.Pets[i].ProgressionPackets(target, s.Assets.Items, s.petTemplate(next.Pets[i].ID)))
	}
	next := c.character.Clone()
	if !next.Allocate(requests) {
		return nil
	}
	full := next.Combat(s.Assets.Items)
	next.MaxHP, next.MaxSP = uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
	next.HP, next.SP = min(next.HP, next.MaxHP), min(next.SP, next.MaxSP)
	unlocks := s.unlockQualifiedSkills(&next, true)
	if e := s.commit(ctx, c, next); e != nil {
		return e
	}
	return s.sendAll(c, append(unlocks, next.StatPackets(s.Assets.Items)...))
}

const (
	starItem = 30025
	tentItem = 36002
)

// useItem is AC23.Recv96: equipment is worn, the Star explains itself, and anything
// else uses the recovery, gacha, voucher or tent workflow.
func (s *Server) useItem(ctx context.Context, c *Session, slot byte) error {
	if handled, err := s.redeemItemCard(ctx, c, slot); handled || err != nil {
		return err
	}
	if slot < 1 || slot > game.BagSize || c.character.Bag[slot-1].Empty() {
		return nil
	}
	if c.character.Bag[slot-1].Locked {
		return c.send(serverwire.HeadBanner(game.ErrItemLocked.Error()))
	}
	if _, ok := s.Assets.Fishing.Rod(c.character.Bag[slot-1].ID); ok {
		return s.startFishing(ctx, c, slot)
	}
	if handled, err := s.redeemVoucher(ctx, c, slot, 1, 0); handled || err != nil {
		return err
	}
	if handled, err := s.openBoxedItem(ctx, c, slot); handled || err != nil {
		return err
	}
	if handled, err := s.openGachaPack(ctx, c, slot); handled || err != nil {
		return err
	}
	id := c.character.Bag[slot-1].ID
	switch def := s.Assets.Items[id]; {
	case s.Assets.Travel.ReturnScrolls[id].Map > 0:
		dst := s.Assets.Travel.ReturnScrolls[id]
		return s.travelWithItem(ctx, c, slot, false, world.Destination{Map: dst.Map, X: dst.X, Y: dst.Y}, func(item game.Item) bool { return item.ID == id })
	case s.Assets.Travel.Scrolls[id] > 0:
		return nil // The native client opens its world map before AC74.
	case id == tentItem:
		return s.openPlayerTent(ctx, c, slot)
	case def.EquipSlot >= 1 && def.EquipSlot <= 6:
		return s.changeEquipment(ctx, c, slot, 0, true)
	case id == game.NotepadItemID:
		return s.sendAll(c, s.World.Journal(c.character, c.view))
	case id == game.RemoteControlItemID:
		return c.send([]byte{protocol.CommandLegacyStorage, protocol.RemoteControlOpen})
	case def.Type == game.VehicleType:
		if c.character.ActiveVehicle == id {
			return s.vehicleCommand(ctx, c, protocol.Builder{protocol.CommandPetControl, protocol.PetControlDismountVehicle, 0}.U16(id))
		}
		return s.vehicleCommand(ctx, c, protocol.Builder{protocol.CommandPetControl, protocol.PetControlPlaceVehicleManual, slot}.U16(id))
	case id == starItem:
		return c.send(serverwire.SystemLine("Stars are special quest tokens used for skill learning, resets, and rebirth quests."))
	}
	if _, food := s.Assets.PetFoods[id]; food {
		var target byte
		if p := c.character.BattlePet(); p != nil {
			target = c.pets.slot(p.ID)
		}
		_, err := s.usePetStatFood(ctx, c, slot, 1, target)
		return err
	}
	ok, e := s.recover(ctx, c, slot, 1, 0)
	if e != nil || ok {
		return e
	}
	return c.send(serverwire.HeadBanner("Select a suitable target to use this item."))
}

// useItemOn is AC23.Recv15: use count items on a target (0 is the character).
func (s *Server) useItemOn(ctx context.Context, c *Session, slot, count byte, target uint16) error {
	if slot >= 1 && slot <= game.BagSize && (s.Assets.Items[c.character.Bag[slot-1].ID].Type == game.ItemTypeRedemptionCard || len(s.Assets.ItemCards[c.character.Bag[slot-1].ID].Rewards) > 0) {
		if count == 1 && target == 0 {
			_, err := s.redeemItemCard(ctx, c, slot)
			return err
		}
		return nil
	}
	if slot < 1 || slot > game.BagSize {
		return nil
	}
	if _, ok := s.Assets.Fishing.Rod(c.character.Bag[slot-1].ID); ok {
		if count == 1 && target == 0 {
			return s.startFishing(ctx, c, slot)
		}
		return nil
	}
	if c.character.Bag[slot-1].ID == tentItem {
		if count == 1 && target == 0 {
			return s.openPlayerTent(ctx, c, slot)
		}
		return nil
	}
	if c.character.Bag[slot-1].Locked {
		return c.send(serverwire.HeadBanner(game.ErrItemLocked.Error()))
	}
	id := c.character.Bag[slot-1].ID
	def := s.Assets.Items[id]
	if count == 1 && target == 0 && (id == game.NotepadItemID || id == game.RemoteControlItemID || def.Type == game.VehicleType || def.EquipSlot >= 1 && def.EquipSlot <= 6 || s.Assets.Travel.Scrolls[id] > 0 || s.Assets.Travel.ReturnScrolls[id].Map > 0) {
		return s.useItem(ctx, c, slot)
	}
	if _, boxed := s.Assets.BoxedItems[id]; boxed {
		if count != 1 || target != 0 {
			return nil
		}
		_, err := s.openBoxedItem(ctx, c, slot)
		return err
	}
	if s.Assets.IsGachaPack(id) {
		if count != 1 || target != 0 {
			return c.send(serverwire.HeadBanner("Open one pack at a time on your character."))
		}
		_, err := s.openGachaPack(ctx, c, slot)
		return err
	}

	if target <= game.MaxPets {
		if handled, err := s.usePetStatFood(ctx, c, slot, count, byte(target)); handled || err != nil {
			return err
		}
	}
	if handled, err := s.redeemVoucher(ctx, c, slot, count, target); handled || err != nil {
		return err
	}
	ok, e := s.recover(ctx, c, slot, count, target)
	if e != nil || ok {
		return e
	}
	return c.send(serverwire.HeadBanner("This item cannot benefit the selected target right now."))
}

// recover is TryUseRecoveryItem for the character: each item restores its HP/SP status
// values above 100, capped at the equipped maxima; nothing is used when already full.
func (s *Server) recover(ctx context.Context, c *Session, slot, count byte, target uint16) (bool, error) {
	if slot < 1 || slot > game.BagSize || count == 0 || target > 4 {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	def, ok := s.Assets.Items[item.ID]
	if item.Empty() || !ok || (def.EquipSlot >= 1 && def.EquipSlot <= 6) {
		return false, nil
	}
	if def.Status[0] == 64 || def.Status[1] == 64 {
		return s.feedPet(ctx, c, slot, count, byte(target))
	}
	if target != 0 {
		return s.recoverPet(ctx, c, slot, count, byte(target), def)
	}
	var hpGain, spGain int64
	for i, status := range def.Status {
		gain := int64(max(0, def.Values[i]-100))
		switch status {
		case 25, 207:
			hpGain += gain
		case 26, 208:
			spGain += gain
		}
	}
	next := c.character.Clone()
	full := next.Combat(s.Assets.Items)
	maxHP, maxSP := int64(max(full.MaxHP, 1)), int64(max(full.MaxSP, 0))
	hp, sp := int64(next.HP), int64(next.SP)
	if (hpGain <= 0 && spGain <= 0) || ((hpGain <= 0 || hp >= maxHP) && (spGain <= 0 || sp >= maxSP)) {
		return false, nil
	}
	used := min(count, item.Count)
	if e := next.Bag.Remove(slot, used); e != nil {
		return false, nil
	}
	next.MaxHP, next.MaxSP = uint32(maxHP), uint32(maxSP)
	next.HP = uint32(min(maxHP, hp+hpGain*int64(used)))
	next.SP = uint32(min(maxSP, sp+spGain*int64(used)))
	if e := s.commit(ctx, c, next); e != nil {
		return false, e
	}
	packets := append([][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, used}}, next.StatPackets(s.Assets.Items)...)
	// AC23:15 plays the use sound.
	return true, s.sendAll(c, append(packets, []byte{protocol.CommandInventory, protocol.InventoryItemUse}))
}

// destroyItem is AC23.Recv124, the confirmed destruction of an undroppable item.
func (s *Server) destroyItem(ctx context.Context, c *Session, slot, count byte) error {
	if slot < 1 || slot > game.BagSize || count == 0 {
		return nil
	}
	item := c.character.Bag[slot-1]
	if item.Empty() {
		return nil
	}
	removed := min(count, item.Count)
	next := c.character.Clone()
	if e := next.Bag.Remove(slot, removed); e != nil {
		return nil
	}
	if e := s.commit(ctx, c, next); e != nil {
		return e
	}
	return s.sendAll(c, [][]byte{protocol.Builder{protocol.CommandInventory, protocol.InventoryDropped}.U16(item.ID).U8(count), {protocol.CommandInventory, protocol.InventoryRemove, slot, removed}})
}

// clientPet finds the party pet in a client slot.
func (s *Server) clientPet(c *Session, slot byte) int {
	for i, p := range c.character.Pets {
		if slot != 0 && c.pets.slot(p.ID) == slot {
			return i
		}
	}
	return -1
}

// feedPet is PetAmityManager.TryFeedPet: pet food raises amity by its status-64 value
// above 100, using only as many items as reach 100.
func (s *Server) feedPet(ctx context.Context, c *Session, slot, count, target byte) (bool, error) {
	if slot < 1 || slot > game.BagSize || count == 0 || c.pets == nil {
		return false, nil
	}
	i := s.clientPet(c, target)
	item := c.character.Bag[slot-1]
	if target < 1 || i < 0 || item.Empty() || c.character.Pets[i].Amity >= petFeedingAmityCap {
		return false, nil
	}
	def := s.Assets.Items[item.ID]
	gain := 0
	for k, status := range def.Status {
		if status == petFeedingAmityStatus {
			gain += max(0, int(def.Values[k])-petFeedingNativeValueOffset)
		}
	}
	if gain <= 0 {
		return false, nil
	}
	pet := c.character.Pets[i]
	needed := (petFeedingAmityCap - int(pet.Amity) + gain - 1) / gain
	used := byte(min(needed, int(count), int(item.Count)))
	next := c.character.Clone()
	if next.Bag.Remove(slot, used) != nil {
		return false, nil
	}
	before := pet.Amity
	next.Pets[i].Amity = byte(min(petFeedingAmityCap, int(before)+gain*int(used)))
	if e := s.commit(ctx, c, next); e != nil {
		return false, e
	}
	after := next.Pets[i].Amity
	return true, s.sendAll(c, [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, used}, game.PetStat(target, petFeedingAmityStatus, int64(after)), {protocol.CommandInventory, protocol.InventoryItemUse},
		serverwire.HeadBanner(fmt.Sprintf("%s: Amity +%d (%d/%d)", s.companionName(pet.ID, pet.Name), after-before, after, petFeedingAmityCap))})
}

// recoverPet is TryUseRecoveryItem for a pet target.
func (s *Server) recoverPet(ctx context.Context, c *Session, slot, count, target byte, def game.ItemDefinition) (bool, error) {
	i := s.clientPet(c, target)
	if i < 0 {
		return false, nil
	}
	var hpGain, spGain int64
	for k, status := range def.Status {
		gain := int64(max(0, def.Values[k]-100))
		switch status {
		case 25, 207:
			hpGain += gain
		case 26, 208:
			spGain += gain
		}
	}
	next := c.character.Clone()
	pet := &next.Pets[i]
	pet.Normalize(s.Assets.Items, false, s.petTemplate(pet.ID))
	hp, sp := int64(pet.HP), int64(pet.SP)
	if (hpGain <= 0 && spGain <= 0) || ((hpGain <= 0 || hp >= int64(pet.MaxHP)) && (spGain <= 0 || sp >= int64(pet.MaxSP))) {
		return false, nil
	}
	used := min(count, c.character.Bag[slot-1].Count)
	if next.Bag.Remove(slot, used) != nil {
		return false, nil
	}
	pet.HP = int32(min(int64(pet.MaxHP), hp+hpGain*int64(used)))
	pet.SP = int32(min(int64(pet.MaxSP), sp+spGain*int64(used)))
	if e := s.commit(ctx, c, next); e != nil {
		return false, e
	}
	packets := append([][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, used}}, pet.ProgressionPackets(target, s.Assets.Items, s.petTemplate(pet.ID))...)
	return true, s.sendAll(c, append(packets, []byte{protocol.CommandInventory, protocol.InventoryItemUse}))
}

// openBoxedItem atomically exchanges a deterministic box for its SQL-defined
// contents. Failed delivery retains the box, including its instance metadata.
func (s *Server) openBoxedItem(ctx context.Context, c *Session, slot byte) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	box, known := s.Assets.BoxedItems[item.ID]
	if !known {
		return false, nil
	}
	if item.Locked {
		return true, c.send(serverwire.HeadBanner(game.ErrItemLocked.Error()))
	}
	if item.Empty() || c.event != nil || c.battle != nil || c.storm || c.beach != nil {
		return true, nil
	}
	limit, known := s.stackLimit(box.ItemID)
	if !known || box.Quantity == 0 {
		return true, c.send(serverwire.HeadBanner("This box's contents are unavailable. Box retained."))
	}
	var adds []game.Addition
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		current := next.Bag[slot-1]
		if current.ID != item.ID || current.Empty() || current.Locked {
			return game.ErrInvalidItem
		}
		if err := next.Bag.Remove(slot, 1); err != nil {
			return err
		}
		var err error
		adds, err = next.Bag.Grant(game.Item{ID: box.ItemID}, int(box.Quantity), limit, s.Assets.Items)
		return err
	}, *c.character)
	if errors.Is(err, game.ErrInventoryFull) || errors.Is(err, game.ErrInvalidItem) {
		return true, c.send(serverwire.HeadBanner("Cannot open this box. Check inventory space and ownership."))
	}
	if err != nil {
		return true, err
	}
	s.adoptSavedCharacter(c, next)
	return true, s.sendAll(c, [][]byte{
		{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, next.Bag.AdditionPacket(adds),
		{protocol.CommandInventory, protocol.InventoryItemUse},
		serverwire.HeadBanner(fmt.Sprintf("Received [%s] x%d.", s.itemName(box.ItemID), box.Quantity)),
	})
}

// redeemVoucher is PetVoucherManager.TryRedeem. A recognized voucher is handled
// even on refusal. Bag consumption and the new pet are committed together.
func (s *Server) redeemVoucher(ctx context.Context, c *Session, slot, count byte, target uint16) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	id, known := s.Assets.PetVouchers[item.ID]
	if !known {
		return false, nil
	}
	refuse := func(text string) (bool, error) { return true, c.send(serverwire.HeadBanner(text)) }
	if count != 1 || target != 0 {
		return refuse("Use one pet voucher at a time on your character.")
	}
	if item.Empty() || c.battle != nil {
		return refuse("Cannot use this pet voucher right now. Voucher retained.")
	}
	t, available := s.World.Template(uint32(id))
	if !available || !s.hasItem(item.ID) {
		return refuse("This pet has no available data. Voucher retained.")
	}
	if _, owned := c.character.Pet(uint32(id)); owned {
		return refuse("This pet is already in your party. Voucher retained.")
	}
	next := c.character.Clone()
	partySlot := next.FreePetSlot()
	if partySlot == 0 {
		return refuse("Your pet party is full. Free a slot first. Voucher retained.")
	}
	// Reserve the client slot on a copy, so a failed database commit cannot
	// consume a per-login roster slot. Hotel copies are allowed by the source.
	roster := &petRoster{slots: make(map[uint32]byte), synced: c.pets.synced}
	for id, slot := range c.pets.slots {
		roster.slots[id] = slot
	}
	if !roster.register(uint32(id)) {
		return refuse("No free pet slot is available. Voucher retained.")
	}
	pet := game.NewPet(uint32(id), s.npcName(uint32(id)), partySlot, t, s.Assets.Items)
	pet.EnsureSkills(t, s.hasSkill)
	next.Pets = append(next.Pets, pet)
	if err := next.Bag.Remove(slot, 1); err != nil {
		return true, err
	}
	if err := s.commit(ctx, c, next); err != nil {
		return true, err
	}
	c.pets = roster
	cs := roster.slot(pet.ID)
	packets := [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, pet.RecruitPacket(next.ID, t)}
	packets = append(packets, pet.ProgressionPackets(cs, s.Assets.Items, s.petTemplate(pet.ID))...)
	packets = append(packets, serverwire.PetNamePacket(next.ID, cs, pet.Name), []byte{protocol.CommandInventory, protocol.InventoryItemUse}, serverwire.HeadBanner(pet.Name+" joined your party."))
	if pet.Equipment != (game.Equipment{}) {
		packets = append(packets, s.petListPacket(c.character, c.pets))
	}
	return true, s.sendAll(c, packets)
}

// repairCommand ports AC36 and EquipmentRepairManager. The C# manager charges
// payment without clearing damage; Go fixes that and saves before any receipts.
// AC36 addresses the bag, despite the reference handler's worn-equipment summary.
// Caller holds worldMu; the world dispatcher guards loading, battle and trade.
func (s *Server) repairCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.RepairRequestBytes {
		return protocol.ErrMalformed
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	slot := p[2]
	reply := []byte{protocol.CommandEquipmentRepair, p[1], slot, protocol.RepairFailed}
	if slot < 1 || slot > game.BagSize {
		return c.send(reply)
	}
	item := c.character.Bag[slot-1]
	if item.Empty() {
		return s.sendAll(c, [][]byte{serverwire.SystemLine("Please select an item in your inventory to repair."), reply})
	}
	if _, known := s.Assets.Items[item.ID]; !known {
		return c.send(reply)
	}
	if item.Damage == 0 {
		return c.send(reply)
	}
	if c.character.ActiveVehicle != 0 && c.character.VehicleSlot == slot {
		return nil
	}
	next := c.character.Clone()
	tool, ok := next.RepairBagItem(slot)
	if !ok {
		return s.sendAll(c, [][]byte{serverwire.SystemLine("You need a Repair Wrench or 500 gold to repair your equipment!"), reply})
	}
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	var packets [][]byte
	if tool != 0 {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, tool, game.RepairToolsPerItem})
	} else {
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
	}
	// AC23:5 is additive. Remove the client's remaining old stack before adding
	// its repaired record, preserving forge metadata and avoiding duplication.
	count := next.Bag[slot-1].Count
	packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, count}, next.Bag.AdditionPacket([]game.Addition{{Slot: slot, Count: count}}))
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	s.clearShapeshift(c)
	reply[3] = protocol.RepairSucceeded
	return s.sendAll(c, [][]byte{serverwire.SystemLine(fmt.Sprintf("[Blacksmith Repair] Successfully restored item #%d to maximum durability!", item.ID)), reply})
}
