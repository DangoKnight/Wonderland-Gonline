package server

import (
	"context"
	"math/rand/v2"
	"slices"
	"strings"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
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

func petNamePacket(owner uint32, slot byte, name string) []byte {
	return protocol.Builder{protocol.CommandPetControl, protocol.PetControlBoardVehicleAlternate}.U32(owner).U8(slot).Bytes([]byte(name))
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
		pet.Normalize(s.Assets.Items, false)
		p = pet.ListRecord(p, slot, s.companionName(pet.ID, pet.Name), s.petTemplate(pet.ID))
		n++
	}
	if n == 0 {
		return nil
	}
	return p
}

// petMapPacket is CreatePetMapPacket (AC15:4), the pet another player sees beside its owner.
func petMapPacket(owner, pet uint32, name string) []byte {
	p, _ := protocol.Builder{protocol.CommandPetControl, protocol.PetControlWireCode4}.U32(owner).U32(pet).U8(0).U8(1).String(name)
	return p.U16(0).U16(0).U8(0).U8(0).U16(0)
}

// peerPetPackets is SendPeerCompanionAndVehicle for pets and companion mounts.
// Item vehicles precede companion appearance, as in the native map snapshot.
func (s *Server) peerPetPackets(char *game.Character) [][]byte {
	var packets [][]byte
	if _, ok := char.Vehicle(char.VehicleSlot, char.ActiveVehicle, s.Assets.Items); ok {
		packets = append(packets, vehicleMountPacket(char))
	}
	if pet := mountedPet(char); pet != nil {
		packets = append(packets, mountPacket(char.ID, game.BroadcastID(pet.ID), 1))
	}
	pet := char.BattlePet()
	if pet == nil {
		return packets
	}
	id := game.BroadcastID(pet.ID)
	return append(packets, petMapPacket(char.ID, id, s.companionName(pet.ID, pet.Name)), protocol.Builder{protocol.CommandTeam, protocol.TeamFormation}.U32(char.ID).U32(id), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
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
		pet.Normalize(s.Assets.Items, false)
		out = append(out, pet.ProgressionPackets(slot, s.Assets.Items)...)
		out = append(out, petNamePacket(char.ID, slot, s.companionName(pet.ID, pet.Name)))
		if battle := char.BattlePet(); battle != nil && game.SamePet(battle.ID, pet.ID) {
			id := game.BroadcastID(pet.ID)
			out = append(out, protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(id), protocol.Builder{protocol.CommandTeam, protocol.TeamFormation}.U32(char.ID).U32(id), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
		}
	}
	if pet := mountedPet(char); pet != nil {
		out = append(out, mountPacket(char.ID, game.BroadcastID(pet.ID), pet.Slot), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSpriteRefresh}.U32(char.ID).U8(0))
	}
	return out
}

// petCommand is AC19: 1/4 selects the battle pet by client slot or pet ID, 2/5 rests it.
// Caller holds worldMu.
func (s *Server) petCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
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
		s.broadcastWorld(c, petMapPacket(next.ID, next.ActivePet, name))
		slot := c.pets.slot(pet.ID)
		packets := [][]byte{protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(next.ActivePet), petNamePacket(next.ID, slot, name)}
		packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items)...)
		return s.sendAll(c, append(packets, systemLine(name+" is now in Battle Mode!")))
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
	return !(game.SamePet(id, 14156) && xaolanInFate(char)) && len(char.Pets) < game.MaxPets
}

func xaolanInFate(c *game.Character) bool {
	q13087, ok1 := c.Quests[13087]
	q13086, ok2 := c.Quests[13086]
	return (ok1 && q13087.State == game.InProgress && q13087.Step > 0) || (ok2 && q13086.State == game.InProgress && q13086.Step >= 2)
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
		return false, c.send(headBanner("This companion is in Pet Hotel. Retrieve it before continuing."))
	}
	if _, known := s.World.Template(game.BroadcastID(id)); !known || (game.SamePet(id, 14156) && xaolanInFate(c.character)) {
		return false, nil
	}
	if id == 12178 {
		id = 12032
	}
	if _, owned := c.character.Pet(id); owned {
		return true, nil
	}
	next := c.character.Clone()
	slot := next.FreePetSlot()
	if slot == 0 {
		return false, c.send(headBanner("No free companion slot available."))
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
		switch {
		case (id == 14161 || id == 14162) && s.hasItem(10063):
			pet.Equipment[2] = game.Item{ID: 10063, Count: 1} // Roca's machete.
		case id == 14156 && s.hasItem(25028):
			pet.Equipment[5] = game.Item{ID: 25028, Count: 1} // Xaolan's jade.
		}
		pet.EnsureSkills(t, s.hasSkill)
		pet.Normalize(s.Assets.Items, true)
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
	packets = append(packets, pet.ProgressionPackets(cs, s.Assets.Items)...)
	packets = append(packets, petNamePacket(next.ID, cs, s.companionName(pet.ID, pet.Name)))
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
		packets = append(packets, unmountPackets(next.ID)...)
		for _, packet := range unmountPackets(next.ID) {
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
