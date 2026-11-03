package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

// petRoster is the client's pet slot numbering for one login (C# PlayerPetData.ClientSlot
// and PetRosterSynchronized). The client rejects duplicate templates and has four slots.
type petRoster struct {
	slots  map[uint32]byte
	synced bool
}

func newPetRoster() *petRoster { return &petRoster{slots: map[uint32]byte{}} }

func (r *petRoster) slot(id uint32) byte {
	for pet, s := range r.slots {
		if game.SamePet(pet, id) {
			return s
		}
	}
	return 0
}

// register is RegisterClientPet: the lowest free client slot, once per companion.
func (r *petRoster) register(id uint32) bool {
	if r.slot(id) != 0 {
		return false
	}
	for s := byte(1); s <= game.MaxPets; s++ {
		used := false
		for _, have := range r.slots {
			used = used || have == s
		}
		if !used {
			r.slots[id] = s
			return true
		}
	}
	return false
}

func (r *petRoster) release(id uint32) byte {
	for pet, s := range r.slots {
		if game.SamePet(pet, id) {
			delete(r.slots, pet)
			return s
		}
	}
	return 0
}

// companionName is ResolveCompanionName: a stored name unless it is a placeholder.
func (s *Server) companionName(id uint32, stored string) string {
	name := strings.TrimSpace(stored)
	if name != "" && name != "Wild Monster" && name != "Wild Monst" && name != "Companion" && !strings.HasPrefix(name, "Companion #") && !strings.HasPrefix(name, "Template #") {
		return name
	}
	return s.npcName(id)
}

func (s *Server) petTemplate(id uint32) game.PetTemplate {
	t, _ := s.World.Template(game.BroadcastID(id))
	return t
}

// petListPacket is CreatePetListPacket (AC15:8) for every registered pet.
func (s *Server) petListPacket(char *game.Character, roster *petRoster) []byte {
	pets := slices.Clone(char.Pets)
	slices.SortFunc(pets, func(a, b game.Pet) int { return int(roster.slot(a.ID)) - int(roster.slot(b.ID)) })
	p := protocol.Builder{protocol.CommandPetControl, protocol.PetControlWireCode8}
	n := 0
	for _, pet := range pets {
		slot := roster.slot(pet.ID)
		if slot == 0 {
			continue
		}
		pet.Normalize(s.Assets.Items, false, s.petTemplate(pet.ID))
		p = pet.ListRecord(p, slot, s.companionName(pet.ID, pet.Name), s.petTemplate(pet.ID))
		n++
	}
	if n == 0 {
		return nil
	}
	return p
}

// peerPetPackets is SendPeerCompanionAndVehicle for pets and companion mounts.
// Item vehicles precede companion appearance, as in the native map snapshot.
func (s *Server) peerPetPackets(char *game.Character) [][]byte {
	var packets [][]byte
	if _, ok := char.Vehicle(char.VehicleSlot, char.ActiveVehicle, s.Assets.Items); ok {
		packets = append(packets, serverwire.VehicleMountPacket(char))
	}
	if pet := mountedPet(char); pet != nil {
		packets = append(packets, serverwire.MountPacket(char.ID, game.BroadcastID(pet.ID), 1))
	}
	pet := char.BattlePet()
	if pet == nil {
		return packets
	}
	id := game.BroadcastID(pet.ID)
	return append(packets, serverwire.PetMapPacket(char.ID, id, s.companionName(pet.ID, pet.Name)), protocol.Builder{protocol.CommandTeam, protocol.TeamFormation}.U32(char.ID).U32(id), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
}

// rosterPackets is Warp_In's owner-side pet synchronization: the first entry of a login
// registers the party and sends AC15:8; every entry refreshes progress and names, and
// selects the battle pet.
func (s *Server) rosterPackets(char *game.Character, roster *petRoster) [][]byte {
	pets := slices.Clone(char.Pets)
	slices.SortFunc(pets, func(a, b game.Pet) int { return int(a.Slot) - int(b.Slot) })
	var out [][]byte
	if !roster.synced && len(pets) > 0 {
		for _, pet := range pets {
			roster.register(pet.ID)
		}
		if p := s.petListPacket(char, roster); p != nil {
			out = append(out, p)
		}
	}
	roster.synced = true
	for _, pet := range pets {
		slot := roster.slot(pet.ID)
		if slot == 0 {
			continue
		}
		pet.Normalize(s.Assets.Items, false, s.petTemplate(pet.ID))
		out = append(out, pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))...)
		out = append(out, serverwire.PetNamePacket(char.ID, slot, s.companionName(pet.ID, pet.Name)))
		if battle := char.BattlePet(); battle != nil && game.SamePet(battle.ID, pet.ID) {
			id := game.BroadcastID(pet.ID)
			out = append(out, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(id), protocol.Builder{protocol.CommandTeam, protocol.TeamFormation}.U32(char.ID).U32(id), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
		}
	}
	if pet := mountedPet(char); pet != nil {
		out = append(out, serverwire.MountPacket(char.ID, game.BroadcastID(pet.ID), pet.Slot), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
	}
	return out
}

// petCommand is AC19: 1/4 selects the battle pet by client slot or pet ID, 2/5 rests it.
// Caller holds worldMu.
func (s *Server) petCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if c.battle != nil {
		return s.battlePetCommand(ctx, c, p)
	}
	switch p[1] {
	case protocol.BattlePetSelect, protocol.BattlePetSelectAlternate:
		data := p[2:]
		var id uint32
		switch {
		case len(data) >= 4:
			id = uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
		case len(data) >= 2:
			id = uint32(data[0]) | uint32(data[1])<<8
		case len(data) == 1:
			id = uint32(data[0])
		}
		chosen := -1
		for i, pet := range c.character.Pets {
			slot := c.pets.slot(pet.ID)
			if slot != 0 && ((id >= 1 && id <= 4 && uint32(slot) == id) || game.SamePet(pet.ID, id)) {
				chosen = i
				if id >= 1 && id <= 4 && uint32(slot) == id {
					break
				}
			}
		}
		if chosen < 0 {
			return nil
		}
		next := c.character.Clone()
		for i := range next.Pets {
			next.Pets[i].Battle = i == chosen
		}
		pet := next.Pets[chosen]
		next.ActivePet = game.BroadcastID(pet.ID)
		if e := s.commit(ctx, c, next); e != nil {
			return e
		}
		name := s.companionName(pet.ID, pet.Name)
		s.broadcastWorld(c, serverwire.PetMapPacket(next.ID, next.ActivePet, name))
		slot := c.pets.slot(pet.ID)
		packets := [][]byte{protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(next.ActivePet), serverwire.PetNamePacket(next.ID, slot, name)}
		packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))...)
		return s.sendAll(c, append(packets, serverwire.SystemLine(name+" is now in Battle Mode!")))
	case protocol.BattlePetRest, protocol.BattlePetRestAlternate:
		next := c.character.Clone()
		next.ActivePet = 0
		for i := range next.Pets {
			next.Pets[i].Battle = false
		}
		if e := s.commit(ctx, c, next); e != nil {
			return e
		}
		refresh := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U8(0)
		s.broadcastWorld(c, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(next.ID))
		s.broadcastWorld(c, refresh)
		return s.sendAll(c, [][]byte{{protocol.CommandBattlePet, protocol.BattlePetRest}, refresh})
	}
	return nil
}

// canRecruit is the companion part of CanDeliverPendingRewards for one recruit.
func (s *Server) canRecruit(char *game.Character, id uint32) bool {
	if slices.ContainsFunc(char.HotelPets, func(p game.Pet) bool { return game.SamePet(p.ID, id) }) {
		return false
	}
	if _, known := s.World.Template(game.BroadcastID(id)); !known {
		return false
	}
	if _, owned := char.Pet(id); owned {
		return true
	}
	return !s.recruitBlocked(char, id) && len(char.Pets) < game.MaxPets
}

func (s *Server) recruitBlocked(c *game.Character, id uint32) bool {
	for r := range s.World.StoryRules(assets.StoryRecruitBlock, c, nil, world.StoryContext{Map: c.Map}) {
		if game.SamePet(id, r.Template) {
			return true
		}
	}
	return false
}

// companionAction is ExecuteCompanionAction (EVE opcode 3): 1 recruits, 2 sends a
// companion away (story companions wait in reserve), 5 changes amity. Caller holds worldMu.
func (s *Server) companionAction(ctx context.Context, c *Session, op world.Op) (bool, error) {
	id := uint32(op.D2)
	switch op.D1 {
	case 1:
		return s.recruit(ctx, c, id)
	case 2:
		return s.dismiss(ctx, c, id)
	case 5:
		i, ok := c.character.Pet(id)
		if !ok {
			return false, nil
		}
		next := c.character.Clone()
		next.Pets[i].Amity = byte(min(100, max(0, int(next.Pets[i].Amity)+int(int8(op.D4>>8)))))
		if e := s.commit(ctx, c, next); e != nil {
			return false, e
		}
		if slot := c.pets.slot(id); slot != 0 {
			return true, c.send(game.PetStat(slot, 64, int64(next.Pets[i].Amity)))
		}
		return true, nil
	}
	return false, nil
}

// recruit is SendCompanionReward without battle selection.
func (s *Server) recruit(ctx context.Context, c *Session, id uint32) (bool, error) {
	if slices.ContainsFunc(c.character.HotelPets, func(p game.Pet) bool { return game.SamePet(p.ID, id) }) {
		return false, c.send(serverwire.HeadBanner("This companion is in Pet Hotel. Retrieve it before continuing."))
	}
	if _, known := s.World.Template(game.BroadcastID(id)); !known || s.recruitBlocked(c.character, id) {
		return false, nil
	}
	if alias, ok := s.Assets.Story.CompanionAliases[id]; ok {
		id = alias
	}
	if _, owned := c.character.Pet(id); owned {
		return true, nil
	}
	next := c.character.Clone()
	slot := next.FreePetSlot()
	if slot == 0 {
		return false, c.send(serverwire.HeadBanner("No free companion slot available."))
	}
	first := true
	var pet game.Pet
	if i := slices.IndexFunc(next.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, id) }); i >= 0 {
		pet, first = next.ReservePets[i], false
		next.ReservePets = slices.Delete(next.ReservePets, i, i+1)
		pet.Slot = slot
	} else {
		t := s.petTemplate(id)
		pet = game.NewPet(id, s.companionName(id, ""), slot, t, s.Assets.Items)
		// A new quest companion arrives with its starter equipment.
		for _, definition := range s.Assets.Story.CompanionEquipment {
			if definition.Template == id {
				for _, equipment := range definition.Equipment {
					if int(equipment.Slot) < len(pet.Equipment) && s.hasItem(equipment.ItemID) {
						pet.Equipment[equipment.Slot] = game.Item{ID: equipment.ItemID, Count: 1}
					}
				}
			}
		}
		pet.EnsureSkills(t, s.hasSkill)
		pet.Normalize(s.Assets.Items, true, s.petTemplate(pet.ID))
	}
	next.Pets = append(next.Pets, pet)
	if e := s.commit(ctx, c, next); e != nil {
		return false, e
	}
	var packets [][]byte
	// The recruited companion's map actor leaves.
	if m, ok := s.World.Map(next.Map); ok {
		for _, n := range m.NPCs {
			if n.Template > 0 && s.World.InParty(c.character, n.Template) {
				packets = append(packets, s.World.HideActor(c.view, next.Map, n.ClickID))
			}
		}
	}
	if c.pets.register(pet.ID) {
		packets = append(packets, pet.RecruitPacket(next.ID, s.petTemplate(pet.ID)))
	}
	cs := c.pets.slot(pet.ID)
	packets = append(packets, pet.ProgressionPackets(cs, s.Assets.Items, s.petTemplate(pet.ID))...)
	packets = append(packets, serverwire.PetNamePacket(next.ID, cs, s.companionName(pet.ID, pet.Name)))
	// AC15:1 has no equipment; AC15:8 restores a returning or equipped companion.
	if !first || pet.Equipment[2].ID != 0 || pet.Equipment[5].ID != 0 {
		if p := s.petListPacket(c.character, c.pets); p != nil {
			packets = append(packets, p)
		}
	}
	return true, s.sendAll(c, packets)
}

// dismiss removes a companion from the party; a story companion keeps its progress
// in reserve until it rejoins.
func (s *Server) dismiss(ctx context.Context, c *Session, id uint32) (bool, error) {
	i, ok := c.character.Pet(id)
	if !ok {
		return true, nil
	}
	next := c.character.Clone()
	pet := next.Pets[i]
	t, known := s.World.Template(game.BroadcastID(pet.ID))
	if game.StoryCompanion(pet.ID, t, known) {
		if slices.ContainsFunc(next.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, pet.ID) }) {
			return false, nil
		}
		reserve := byte(1)
		for slices.ContainsFunc(next.ReservePets, func(p game.Pet) bool { return p.Slot == reserve }) {
			if reserve == 255 {
				return false, nil
			}
			reserve++
		}
		kept := pet
		kept.Slot, kept.Battle = reserve, false
		next.ReservePets = append(next.ReservePets, kept)
	}
	wasActive := next.ActivePet != 0 && game.SamePet(next.ActivePet, pet.ID)
	wasMounted := next.ActiveMount != 0 && game.SamePet(next.ActiveMount, pet.ID)
	if wasMounted {
		next.ActiveMount = 0
	}
	if wasActive {
		next.ActivePet = 0
	}
	next.Pets = slices.Delete(next.Pets, i, i+1)
	if e := s.commit(ctx, c, next); e != nil {
		return false, e
	}
	var packets [][]byte
	if wasMounted {
		packets = append(packets, serverwire.UnmountPackets(next.ID)...)
		for _, packet := range serverwire.UnmountPackets(next.ID) {
			s.broadcastWorld(c, packet)
		}
	}
	if wasActive {
		packets = append(packets, []byte{protocol.CommandBattlePet, protocol.BattlePetRest})
		s.broadcastWorld(c, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(next.ID))
		despawn := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U32(0)
		if next.ActiveMount == 0 {
			packets = append(packets, despawn)
			s.broadcastWorld(c, despawn)
		}
	}
	removed := protocol.Builder{protocol.CommandPetControl, protocol.PetControlPetSlot}.U32(next.ID).U8(c.pets.release(pet.ID))
	s.broadcastWorld(c, removed)
	return true, s.sendAll(c, append(packets, removed))
}

func (s *Server) hasItem(id uint16) bool  { _, ok := s.Assets.Items[id]; return ok }
func (s *Server) hasSkill(id uint16) bool { _, ok := s.Assets.Skills[id]; return ok }

// petGrowth is the weighted roll for a pet's level-up stat.
func petGrowth(n int) int { return rand.IntN(n) }

// gainPetExp is called with worldMu held to serialize pet progression with gameplay mutations.
func (s *Server) gainPetExp(pet *game.Pet, amount uint32, roll func(int) int) int {
	template, known := s.World.Template(game.BroadcastID(pet.ID))
	return pet.GainExp(amount, template, known, roll, game.PetGrowthOptions{Formula: s.petGrowthFormula, Items: s.Assets.Items})
}

func (s *Server) petControlCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.PetControlBoardVehicle, protocol.PetControlBoardVehicleAlternate, protocol.PetControlDismountVehicle, protocol.PetControlVehicleNoOp, protocol.PetControlPlaceVehicle:
		return s.vehicleCommand(ctx, c, p)
	case protocol.PetControlPetSlot:
		return s.releasePet(ctx, c, p)
	case protocol.PetControlRename:
		return s.renamePet(ctx, c, p)
	default:
		return s.mountCommand(ctx, c, p)
	}
}

// renamePet is AC15:6: client slot followed by raw ASCII bytes (not a
// length-prefixed string). Match .NET ASCII replacement, trimming and 16-byte cap.
func (s *Server) renamePet(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 4 {
		return protocol.ErrMalformed
	}
	slot := p[2]
	i := slices.IndexFunc(c.character.Pets, func(p game.Pet) bool { return slot != 0 && c.pets.slot(p.ID) == slot })
	if i < 0 {
		return nil
	}
	raw := append([]byte(nil), p[3:]...)
	for i, b := range raw {
		if b >= 128 {
			raw[i] = '?'
		}
	}
	name := strings.TrimSpace(strings.TrimRight(string(raw), "\x00"))
	name = name[:min(16, len(name))]
	if name == "" {
		return nil
	}
	for _, b := range []byte(name) {
		if b < 32 || b == 127 {
			return nil
		}
	}
	if name == c.character.Pets[i].Name {
		return nil
	}
	next := c.character.Clone()
	next.Pets[i].Name = name
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	pet := next.Pets[i]
	if pet.Battle || (next.ActivePet != 0 && game.SamePet(next.ActivePet, pet.ID)) {
		s.broadcastWorld(c, serverwire.PetMapPacket(next.ID, game.BroadcastID(pet.ID), name))
	}
	return c.send(serverwire.PetNamePacket(next.ID, slot, name))
}

// releasePet permanently releases the client-selected party pet. EVE dismiss
// remains separate: story companions temporarily dismissed there enter reserve.
func (s *Server) releasePet(ctx context.Context, c *Session, p []byte) error {
	if len(p) != 3 {
		return protocol.ErrMalformed
	}
	slot := p[2]
	i := slices.IndexFunc(c.character.Pets, func(p game.Pet) bool { return slot != 0 && c.pets.slot(p.ID) == slot })
	if i < 0 {
		return nil
	}
	next := c.character.Clone()
	pet := next.Pets[i]
	var packets, peers [][]byte
	if next.ActiveMount != 0 && game.SamePet(next.ActiveMount, pet.ID) {
		next.ActiveMount = 0
		packets = append(packets, serverwire.UnmountPackets(next.ID)...)
		peers = append(peers, serverwire.UnmountPackets(next.ID)...)
	}
	if pet.Battle || (next.ActivePet != 0 && game.SamePet(next.ActivePet, pet.ID)) {
		next.ActivePet = 0
		packets = append(packets, []byte{protocol.CommandBattlePet, protocol.BattlePetRest})
		peers = append(peers, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetWireCode7}.U32(next.ID))
		if next.ActiveMount == 0 {
			despawn := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U32(0)
			packets = append(packets, despawn)
			peers = append(peers, despawn)
		}
	}
	next.Pets = slices.Delete(next.Pets, i, i+1)
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	c.pets.release(pet.ID)
	removed := protocol.Builder{protocol.CommandPetControl, protocol.PetControlPetSlot}.U32(next.ID).U8(slot)
	packets = append(packets, removed)
	peers = append(peers, removed)
	for _, packet := range peers {
		s.broadcastWorld(c, packet)
	}
	packets = append(packets, serverwire.SystemLine(fmt.Sprintf("Released companion pet %s (Slot %d).", pet.Name, slot)))
	return s.sendAll(c, packets)
}

// changePetEquipment is AC23:17/18 and Inventory.TryEquipPet/TryUnequipPet.
// The shared equipment rules retain all item metadata during swaps.
func (s *Server) changePetEquipment(ctx context.Context, c *Session, slot, from, to byte, wear bool) error {
	if c.battle != nil {
		return nil
	}
	i := s.clientPet(c, slot)
	if i < 0 {
		return nil
	}
	next := c.character.Clone()
	pet := &next.Pets[i]
	before := pet.Combat(s.Assets.Items, s.petTemplate(pet.ID))
	transfer := game.Character{Bag: next.Bag, Equipment: pet.Equipment}
	var err error
	packet := []byte{protocol.CommandInventory, protocol.InventoryWireCode23, slot, from}
	if wear {
		if from < 1 || from > game.BagSize {
			return nil
		}
		equipSlot := s.Assets.Items[next.Bag[from-1].ID].EquipSlot
		if equipSlot >= 1 && equipSlot <= 6 {
			previous := pet.Equipment[equipSlot-1]
			if previous.ID != 0 && !s.hasItem(previous.ID) {
				return nil
			}
		}
		err = transfer.Wear(from, s.Assets.Items)
	} else {
		if from < 1 || from > 6 || !s.hasItem(pet.Equipment[from-1].ID) {
			return nil
		}
		err = transfer.Unwear(from, to, s.Assets.Items)
		packet = []byte{protocol.CommandInventory, protocol.InventoryWireCode22, slot, from, to}
	}
	if err != nil {
		return nil
	}
	next.Bag, pet.Equipment = transfer.Bag, transfer.Equipment
	pet.Normalize(s.Assets.Items, false, s.petTemplate(pet.ID))
	after := pet.Combat(s.Assets.Items, s.petTemplate(pet.ID))
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	packets := append([][]byte{packet}, pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))...)
	if text := game.ChangeBanner(pet.Name+" equipment: ", before, after); text != "" {
		packets = append(packets, serverwire.HeadBanner(text))
	}
	return s.sendAll(c, packets)
}

const (
	petFeedingItemsPerRequest   = 1
	petFeedingAmityCap          = 100
	petFeedingAmityStatus       = 64
	petFeedingNativeValueOffset = 100
)

// petFeedingCommand ports AC67 and PetAmityManager.FeedPet. The request names
// an item ID, not a bag slot. Only a registered party pet marked for battle is
// eligible; TryFeedPet's shared consumption policy owns the durable transition.
func (s *Server) petFeedingCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.PetFeedingRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.PetFeedingFeed {
		return ErrUnsupported
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	r := protocol.NewReader(p[2:])
	food := r.U16()
	reply := protocol.Builder{protocol.CommandPetFeeding, protocol.PetFeedingFeed}.U16(food)
	var target byte
	if c.pets != nil {
		for _, pet := range c.character.Pets {
			if pet.ID != 0 && pet.Battle && c.pets.slot(pet.ID) != 0 {
				target = c.pets.slot(pet.ID)
				break
			}
		}
	}
	if target != 0 && food != 0 {
		for index, item := range c.character.Bag {
			if item.ID != food || item.Empty() {
				continue
			}
			ok, err := s.feedPet(ctx, c, byte(index+1), petFeedingItemsPerRequest, target)
			if err != nil {
				return err
			}
			if ok {
				return c.send(reply.U8(protocol.PetFeedingSucceeded))
			}
		}
	}
	return s.sendAll(c, [][]byte{serverwire.HeadBanner("Select a battle pet and use an available amity item."), reply.U8(protocol.PetFeedingFailed)})
}

// petTrainingCommand ports AC68:2. Caller holds worldMu and enforces world
// loading/battle/trade gates. Potential pills use the separate native AC23:126 handler.
func (s *Server) petTrainingCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.PetTrainingRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.PetTrainingAllocate && p[1] != protocol.PetTrainingPotential {
		return ErrUnsupported
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	slot, selector := p[2], p[3]
	reply := []byte{protocol.CommandPetTraining, p[1], slot, selector, protocol.PetTrainingFailed}
	if c.pets == nil {
		return c.send(reply)
	}
	i := s.clientPet(c, slot)
	if i < 0 || c.character.Pets[i].ID == 0 {
		return c.send(reply)
	}
	if p[1] == protocol.PetTrainingPotential {
		return s.sendAll(c, [][]byte{reply, serverwire.SystemLine("Use a Potential Pill from inventory; free potential training is disabled.")})
	}
	var stat byte
	switch selector {
	case protocol.PetTrainingStrength:
		stat = game.StatSTR
	case protocol.PetTrainingConstitution:
		stat = game.StatCON
	case protocol.PetTrainingIntelligence:
		stat = game.StatINT
	case protocol.PetTrainingWisdom:
		stat = game.StatWIS
	case protocol.PetTrainingAgility:
		stat = game.StatAGI
	default:
		return c.send(reply)
	}
	next := c.character.Clone()
	pet := &next.Pets[i]
	if !pet.AllocatePoint(stat) {
		return c.send(reply)
	}
	pet.Normalize(s.Assets.Items, false, s.petTemplate(pet.ID))
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	packets := pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))
	reply[4] = protocol.PetTrainingSucceeded
	return s.sendAll(c, append(packets, reply))
}

// petRebirthCommand ports AC69. The dispatcher holds worldMu and guards loading,
// battle and trade. Ascension is selected by the session's native client slot.
func (s *Server) petRebirthCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.PetRebirthRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.PetRebirthAscend {
		return ErrUnsupported
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	slot := p[2]
	reply := []byte{protocol.CommandPetRebirth, protocol.PetRebirthAscend, slot, protocol.PetRebirthFailed}
	if c.pets == nil || slot == 0 {
		return c.send(reply)
	}
	i := slices.IndexFunc(c.character.Pets, func(pet game.Pet) bool { return pet.ID != 0 && c.pets.slot(pet.ID) == slot })
	if i < 0 {
		return c.send(reply)
	}
	next := c.character.Clone()
	pet := &next.Pets[i]
	if !pet.Rebirth(s.Assets.Items, s.petTemplate(pet.ID)) {
		message := fmt.Sprintf("[Rebirth] %s requires level %d and amity %d.", pet.Name, game.PetRebirthRequiredLevel, game.PetRebirthRequiredAmity)
		if pet.Reborn {
			message = fmt.Sprintf("[Rebirth] %s has already undergone Rebirth ascension.", pet.Name)
		} else if pet.StatPoints > math.MaxUint16-game.PetRebirthBonusStatPoints {
			message = "[Rebirth] Spend some pet stat points before receiving the rebirth bonus."
		}
		return s.sendAll(c, [][]byte{reply, serverwire.SystemLine(message)})
	}
	// Normalize before saving so reconnect sees the exact same full vitals.
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	reply[3] = protocol.PetRebirthSucceeded
	packets := [][]byte{reply, s.petListPacket(c.character, c.pets)}
	packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(pet.ID))...)
	packets = append(packets, serverwire.SystemLine(fmt.Sprintf("[Rebirth] %s has attained Rebirth Ascension!", pet.Name)))
	return s.sendAll(c, packets)
}

// companionRebirthCommand is the native altar operation. Opening its authored
// service authorizes one successful request during this map visit. Commit before
// replying and keep the form usable after a rejected selection or failed save.
func (s *Server) companionRebirthCommand(ctx context.Context, c *Session, slot byte) error {
	reply := func(code byte) error {
		return c.send([]byte{protocol.CommandCompanionRebirth, protocol.CompanionRebirthRequest, code})
	}
	if c.view == nil || !c.view.CompanionRebirthOpen || c.event != nil || c.storm || c.beach != nil {
		return reply(protocol.CompanionRebirthUnavailable)
	}
	if c.pets == nil || slot == 0 {
		return reply(protocol.CompanionRebirthNoPet)
	}
	i := slices.IndexFunc(c.character.Pets, func(p game.Pet) bool { return p.ID != 0 && c.pets.slot(p.ID) == slot })
	if i < 0 {
		return reply(protocol.CompanionRebirthNoPet)
	}
	next := c.character.Clone()
	p := &next.Pets[i]
	if p.Reborn {
		return reply(protocol.CompanionRebirthAlready)
	}
	if p.Level < game.PetRebirthRequiredLevel {
		return reply(protocol.CompanionRebirthLevel)
	}
	if p.Amity < game.CompanionRebirthRequiredAmity {
		return reply(protocol.CompanionRebirthAmity)
	}
	for _, item := range p.Equipment {
		if item.ID != 0 {
			return reply(protocol.CompanionRebirthEquipment)
		}
	}
	if !p.RebirthAtAltar(s.Assets.Items, s.petTemplate(p.ID)) {
		return reply(protocol.CompanionRebirthUnavailable)
	}
	id := p.ID
	denied := errors.New("companion rebirth eligibility changed")
	code := byte(protocol.CompanionRebirthUnavailable)
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(stored *game.Character) error {
		index := slices.IndexFunc(stored.Pets, func(p game.Pet) bool { return p.ID == id })
		if index < 0 {
			code = protocol.CompanionRebirthNoPet
			return denied
		}
		pet := &stored.Pets[index]
		if pet.Reborn {
			code = protocol.CompanionRebirthAlready
			return denied
		}
		if pet.Level < game.PetRebirthRequiredLevel {
			code = protocol.CompanionRebirthLevel
			return denied
		}
		if pet.Amity < game.CompanionRebirthRequiredAmity {
			code = protocol.CompanionRebirthAmity
			return denied
		}
		for _, item := range pet.Equipment {
			if item.ID != 0 {
				code = protocol.CompanionRebirthEquipment
				return denied
			}
		}
		if !pet.RebirthAtAltar(s.Assets.Items, s.petTemplate(pet.ID)) {
			return denied
		}
		i = index
		return nil
	}, *c.character)
	if errors.Is(err, denied) {
		return reply(code)
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	p = &next.Pets[i]
	c.view.CompanionRebirthOpen = false
	packets := [][]byte{{protocol.CommandCompanionRebirth, protocol.CompanionRebirthRequest, protocol.CompanionRebirthSuccess}, s.petListPacket(c.character, c.pets)}
	packets = append(packets, p.ProgressionPackets(slot, s.Assets.Items, s.petTemplate(p.ID))...)
	return s.sendAll(c, packets)
}

func mountedPet(c *game.Character) *game.Pet {
	if c.ActiveMount == 0 {
		return nil
	}
	if i, ok := c.Pet(c.ActiveMount); ok {
		return &c.Pets[i]
	}
	return nil
}

// mountCommand is AC15:11/12. Caller holds worldMu and has checked readiness and
// battle state. Unlike the legacy handler, stale client slot/ID pairs are ignored.
func (s *Server) mountCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if p[1] != 11 && p[1] != 12 {
		return ErrUnsupported
	}
	if len(p) != 7 {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	slot, id := r.U8(), r.U32()
	if r.Err() != nil {
		return r.Err()
	}
	i, ok := c.character.Pet(id)
	if !ok || slot == 0 || c.pets.slot(c.character.Pets[i].ID) != slot {
		return nil
	}
	pet := c.character.Pets[i]
	next := c.character.Clone()
	var packets [][]byte
	if p[1] == 11 {
		next.ActiveVehicle, next.VehicleSlot = 0, 0
		next.ActiveMount = game.BroadcastID(pet.ID)
		if next.ActiveMount == c.character.ActiveMount {
			return nil
		}
		packets = [][]byte{serverwire.MountPacket(next.ID, next.ActiveMount, pet.Slot), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(next.ID).U8(0)}
	} else {
		if next.ActiveMount == 0 || !game.SamePet(next.ActiveMount, pet.ID) {
			return nil
		}
		next.ActiveMount = 0
		packets = serverwire.UnmountPackets(next.ID)
	}
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	for _, packet := range packets {
		s.broadcastWorld(c, packet)
	}
	return s.sendAll(c, packets)
}
