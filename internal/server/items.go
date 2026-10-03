package server

import (
	"context"
	"fmt"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// headBanner is Player.SendHeadBanner: AC2:16 with raw, unprefixed ASCII text.
func headBanner(text string) []byte {
	return protocol.Builder{protocol.CommandChat, protocol.ChatHeadBanner}.U32(0).Bytes([]byte(text))
}

// itemCommand handles bag and ground actions; the caller holds worldMu. Reference: AC23.Recv2/3/10 and
// GameMap.onItemPickup/onItemDrop, Player.WearEQ/unWearEQ. Remaining item operations are pending.
// Older handlers follow the C# reader and ignore trailing bytes. Compound and
// mall and pack-opening handlers require complete requests.
func (s *Server) itemCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.InventoryStallListRequest:
		// No player stalls are implemented. AC23.Recv77 sends an empty list and
		// its completion marker; the native client needs both replies while loading.
		return s.sendAll(c, [][]byte{{protocol.CommandInventory, protocol.InventoryStallList, 0}, {protocol.CommandInventory, protocol.InventoryStallListComplete}})
	case protocol.InventoryOpenPack, protocol.InventoryOpenPackAlternate:
		return s.openPackCommand(ctx, c, p)
	case protocol.InventoryCompound:
		return s.compoundCommand(ctx, c, p)
	case protocol.InventoryMallBalance, protocol.InventoryMallCatalog, protocol.InventoryMallBuy:
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
		if len(p) < 6 {
			return nil
		}
		return s.useItemOn(ctx, c, p[2], p[3], uint16(p[4])|uint16(p[5])<<8)
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
	var adds []game.Addition
	var e error
	if known {
		adds, e = bag.Grant(target.Item, 1, limit)
	}
	// Unknown IDs cannot enter the bag; C# reports them as a full inventory too.
	if !known || e != nil {
		return c.send(headBanner("Your inventory is full."))
	}
	if e = s.saveBag(ctx, c, bag); e != nil {
		return e
	}
	s.World.Take(char.Map, target, time.Now())
	if e = c.send(bag.AdditionPacket(adds)); e != nil {
		return e
	}
	if e = c.send(protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(uint16(target.Slot)).U8(1)); e != nil {
		return e
	}
	s.broadcastWorld(c, protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(uint16(target.Slot)).U8(0))
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
		return c.send(protocol.Builder{protocol.CommandInventory, protocol.InventoryWireCode212, 255, slot}.U16(item.ID).U8(count))
	}
	if count == 0 || count > item.Count {
		return nil
	}
	slots, ok := s.World.FreeGroundSlots(char.Map, int(count))
	if !ok {
		return c.send(headBanner("There is no room to drop these items here."))
	}
	bag := char.Bag
	if e := bag.Remove(slot, count); e != nil {
		return nil
	}
	if e := s.saveBag(ctx, c, bag); e != nil {
		return e
	}
	packet := s.World.Drop(char.Map, slots, item, char.X, char.Y)
	if e := c.send([]byte{protocol.CommandInventory, protocol.InventoryRemove, slot, count}); e != nil {
		return e
	}
	if e := c.send(packet); e != nil {
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
	moved, e := bag.Move(from, to, count, limit)
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
		if peer.character.Map == mapID {
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
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.respawnGround(now)
			s.respawnChests(now)
			s.reviveMonsters(now)
		}
	}
}

// changeEquipment handles AC23:11 (wear bag slot from) and AC23:12 (remove worn slot from
// to bag slot to). C# does not broadcast the new appearance to map peers.
func (s *Server) changeEquipment(ctx context.Context, c *Session, from, to byte, wear bool) error {
	next := *c.character
	before := next.Combat(s.Assets.Items)
	var e error
	reply := []byte{protocol.CommandInventory, protocol.InventoryEquipmentChanged, from, to}
	if wear {
		e = next.Wear(from, s.Assets.Items)
		reply = []byte{protocol.CommandInventory, protocol.InventoryPetEquip, from, from}
	} else {
		e = next.Unwear(from, to)
	}
	if e != nil {
		return nil
	}
	if e = s.Store.UpdateCharacter(ctx, c.account.ID, next.ID, func(char *game.Character) error {
		char.Bag, char.Equipment = next.Bag, next.Equipment
		return nil
	}); e != nil {
		return e
	}
	c.character.Bag, c.character.Equipment = next.Bag, next.Equipment
	packets := append([][]byte{reply}, next.StatPackets(s.Assets.Items)...)
	if text := game.ChangeBanner("Equipment: ", before, next.Combat(s.Assets.Items)); text != "" {
		packets = append(packets, headBanner(text))
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
		next.Pets[i].Normalize(s.Assets.Items, false)
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		return s.sendAll(c, next.Pets[i].ProgressionPackets(target, s.Assets.Items))
	}
	next := c.character.Clone()
	if !next.Allocate(requests) {
		return nil
	}
	full := next.Combat(s.Assets.Items)
	next.MaxHP, next.MaxSP = uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
	next.HP, next.SP = min(next.HP, next.MaxHP), min(next.SP, next.MaxSP)
	unlocks := next.UnlockQualifiedSkills(true, s.hasSkill)
	if e := s.commit(ctx, c, next); e != nil {
		return e
	}
	return s.sendAll(c, append(unlocks, next.StatPackets(s.Assets.Items)...))
}

const (
	starItem = 30025
	tentItem = 36002
)

// systemLine is AC23:57 with a length-prefixed message.
func systemLine(text string) []byte {
	p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryMessage, 0}.String(text)
	return p
}

// useItem is AC23.Recv96: equipment is worn, the Star explains itself, and anything
// else must be a recovery item, gacha pack or pet voucher. Tents remain pending.
func (s *Server) useItem(ctx context.Context, c *Session, slot byte) error {
	if slot < 1 || slot > game.BagSize || c.character.Bag[slot-1].Empty() {
		return nil
	}
	if handled, err := s.redeemVoucher(ctx, c, slot, 1, 0); handled || err != nil {
		return err
	}
	if handled, err := s.openGachaPack(ctx, c, slot); handled || err != nil {
		return err
	}
	id := c.character.Bag[slot-1].ID
	switch def := s.Assets.Items[id]; {
	case id == tentItem:
		return nil
	case def.EquipSlot >= 1 && def.EquipSlot <= 6:
		return s.changeEquipment(ctx, c, slot, 0, true)
	case id == starItem:
		return c.send(systemLine("Stars are special quest tokens used for skill learning, resets, and rebirth quests."))
	}
	ok, e := s.recover(ctx, c, slot, 1, 0)
	if e != nil || ok {
		return e
	}
	return c.send(headBanner("Select a suitable target to use this item."))
}

// useItemOn is AC23.Recv15: use count items on a target (0 is the character).
func (s *Server) useItemOn(ctx context.Context, c *Session, slot, count byte, target uint16) error {
	if slot < 1 || slot > game.BagSize || c.character.Bag[slot-1].ID == tentItem {
		return nil
	}
	if handled, err := s.redeemVoucher(ctx, c, slot, count, target); handled || err != nil {
		return err
	}
	ok, e := s.recover(ctx, c, slot, count, target)
	if e != nil || ok {
		return e
	}
	return c.send(headBanner("This item cannot benefit the selected target right now."))
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
		headBanner(fmt.Sprintf("%s: Amity +%d (%d/%d)", s.companionName(pet.ID, pet.Name), after-before, after, petFeedingAmityCap))})
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
	pet.Normalize(s.Assets.Items, false)
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
	packets := append([][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, used}}, pet.ProgressionPackets(target, s.Assets.Items)...)
	return true, s.sendAll(c, append(packets, []byte{protocol.CommandInventory, protocol.InventoryItemUse}))
}
