package server

import (
	"context"
	"slices"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

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
	if !ok || c.battle != nil {
		return nil
	}
	next := c.character.Clone()
	var outgoing, incoming []game.Pet
	for _, slot := range deposits {
		i := slices.IndexFunc(next.Pets, func(p game.Pet) bool { return c.pets.slot(p.ID) == slot })
		if i < 0 || next.Pets[i].ID == 0 {
			return nil
		}
		outgoing = append(outgoing, next.Pets[i])
		next.Pets = slices.Delete(next.Pets, i, i+1)
	}
	for _, slot := range withdrawals {
		i := slices.IndexFunc(next.HotelPets, func(p game.Pet) bool { return p.Slot == slot })
		if i < 0 || next.HotelPets[i].ID == 0 {
			return nil
		}
		incoming = append(incoming, next.HotelPets[i])
		next.HotelPets = slices.Delete(next.HotelPets, i, i+1)
	}
	if len(next.Pets)+len(incoming) > game.MaxPets || len(next.HotelPets)+len(outgoing) > game.MaxHotelPets {
		return nil
	}
	team := append(slices.Clone(next.Pets), incoming...)
	for i, pet := range team {
		for _, other := range team[:i] {
			if game.SamePet(pet.ID, other.ID) {
				return nil
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
	for _, slot := range withdrawals {
		packets = append(packets, []byte{protocol.CommandNPCService, protocol.NPCServiceWireCode4, slot})
	}
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
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	for _, pet := range outgoing {
		c.pets.release(pet.ID)
	}
	for _, pet := range incoming {
		c.pets.register(pet.ID)
	}
	if mountRemoved {
		packets = append(unmountPackets(next.ID), packets...)
		for _, packet := range unmountPackets(next.ID) {
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
	for _, pet := range incoming {
		packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items)...)
	}
	return s.sendAll(c, append(packets, s.hotelPackets(c.character)...))
}
