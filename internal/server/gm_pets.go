package server

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

const (
	gmPetNameMaxBytes         = 16
	gmRobinsonTemplateID      = 12032
	gmRobinsonBroadcastID     = 12178
	gmRocaTemplateID          = 14161
	gmRocaAlternateTemplateID = 14162
	gmRocaMacheteItemID       = 10063
	gmXaolanTemplateID        = 14156
	gmXaolanJadeItemID        = 25028
)

func gmSelectedPet(c *game.Character) int {
	for i, p := range c.Pets {
		if p.Battle {
			return i
		}
	}
	for i, p := range c.Pets {
		if c.ActivePet != 0 && game.SamePet(p.ID, c.ActivePet) {
			return i
		}
	}
	if len(c.Pets) > 0 {
		return 0
	}
	return -1
}

func (s *Server) gmPetEdit(ctx context.Context, c *Session, command string, args []string) error {
	if command == "pet" {
		return s.gmRecruitPet(ctx, c, args)
	}
	next := c.character.Clone()
	index := gmSelectedPet(&next)
	if index < 0 {
		return s.chatFeedback(c, "No companion or pet is available.")
	}
	if len(args) > 1 {
		return s.chatFeedback(c, "Too many arguments for this pet command.")
	}
	pet := &next.Pets[index]
	var packets [][]byte
	switch command {
	case "amity", "petamity":
		value := uint64(game.PetRebirthRequiredAmity)
		if len(args) == 1 {
			n, err := strconv.ParseUint(args[0], 10, 8)
			if err != nil {
				return s.chatFeedback(c, "Amity must be a number from 0 to 100.")
			}
			value = min(n, game.PetRebirthRequiredAmity)
		}
		pet.Amity = byte(value)
		packets = append(packets, game.PetStat(c.pets.slot(pet.ID), petFeedingAmityStatus, int64(pet.Amity)))
	case "rebirth", "petrebirth":
		if len(args) != 0 {
			return s.chatFeedback(c, "Usage: /rebirth")
		}
		if pet.Reborn {
			return s.chatFeedback(c, "This pet is already reborn.")
		}
		if pet.StatPoints > math.MaxUint16-game.PetRebirthBonusStatPoints {
			return s.chatFeedback(c, "Spend some pet points before rebirth.")
		}
		// GM override bypasses level/amity requirements, but repeated rebirth must
		// not mint the bonus again. Normalize before saving as the native path does.
		pet.Reborn = true
		pet.Level = game.PetRebirthStartingLevel
		pet.Exp = 0
		pet.StatPoints += game.PetRebirthBonusStatPoints
		pet.Amity = game.PetRebirthRequiredAmity
		pet.Normalize(s.Assets.Items, true)
		packets = append(packets, []byte{protocol.CommandPetRebirth, protocol.PetRebirthAscend, c.pets.slot(pet.ID), protocol.PetRebirthSucceeded})
	case "petlvl", "petlevel":
		if len(args) != 1 {
			return s.chatFeedback(c, "Usage: /petlvl <level>")
		}
		level, err := strconv.ParseUint(args[0], 10, 8)
		if err != nil {
			return s.chatFeedback(c, "Invalid pet level.")
		}
		level = min(max(level, 1), game.MaxLevel)
		gained := max(int(level)-int(pet.Level), 0)
		pet.StatPoints = uint16(min(int(pet.StatPoints)+gained*game.StatPointsPerLevel, math.MaxUint16))
		pet.Level = byte(level)
		pet.Exp = 0
		pet.Normalize(s.Assets.Items, true)
	case "petexp":
		if len(args) != 1 {
			return s.chatFeedback(c, "Usage: /petexp <EXP gain>")
		}
		gain, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil {
			return s.chatFeedback(c, "Invalid pet EXP gain.")
		}
		grown := s.gainPetExp(pet, uint32(gain), petGrowth)
		pet.Normalize(s.Assets.Items, grown > 0)
	}
	packets = append(packets, s.petListPacket(&next, c.pets))
	packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items)...)
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, fmt.Sprintf("Updated pet %s (#%d).", pet.Name, pet.ID))
}

func (s *Server) gmRecruitPet(ctx context.Context, c *Session, args []string) error {
	if len(args) == 0 {
		return s.chatFeedback(c, "Usage: /pet <template ID> [name]")
	}
	id, err := strconv.ParseUint(args[0], 10, 16)
	if err != nil || id == 0 {
		return s.chatFeedback(c, "Invalid pet template ID.")
	}
	templateID := uint32(id)
	if templateID == gmRobinsonBroadcastID {
		templateID = gmRobinsonTemplateID
	}
	if _, known := s.World.Template(game.BroadcastID(templateID)); !known {
		return s.chatFeedback(c, "Unknown pet template.")
	}
	if slices.ContainsFunc(c.character.HotelPets, func(p game.Pet) bool { return game.SamePet(p.ID, templateID) }) {
		return s.chatFeedback(c, "Retrieve this companion from Pet Hotel first.")
	}
	next := c.character.Clone()
	index, owned := next.Pet(templateID)
	fresh := false
	if !owned {
		slot := next.FreePetSlot()
		if slot == 0 {
			return s.chatFeedback(c, "No free companion slot available.")
		}
		pet := game.NewPet(templateID, s.companionName(templateID, ""), slot, s.petTemplate(templateID), s.Assets.Items)
		if i := slices.IndexFunc(next.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, templateID) }); i >= 0 {
			pet = next.ReservePets[i]
			pet.Slot = slot
			next.ReservePets = slices.Delete(next.ReservePets, i, i+1)
		} else {
			fresh = true
			switch {
			case (templateID == gmRocaTemplateID || templateID == gmRocaAlternateTemplateID) && s.hasItem(gmRocaMacheteItemID):
				pet.Equipment[2] = game.Item{ID: gmRocaMacheteItemID, Count: 1}
			case templateID == gmXaolanTemplateID && s.hasItem(gmXaolanJadeItemID):
				pet.Equipment[5] = game.Item{ID: gmXaolanJadeItemID, Count: 1}
			}
			pet.EnsureSkills(s.petTemplate(templateID), s.hasSkill)
			pet.Normalize(s.Assets.Items, true)
		}
		next.Pets = append(next.Pets, pet)
		index = len(next.Pets) - 1
	}
	if len(args) > 1 {
		name := strings.Join(args[1:], " ")
		if !validChatText(name, gmPetNameMaxBytes) {
			return s.chatFeedback(c, "Pet names must contain 1-16 printable bytes.")
		}
		next.Pets[index].Name = name
	}
	for i := range next.Pets {
		next.Pets[i].Battle = i == index
	}
	next.ActivePet = game.BroadcastID(next.Pets[index].ID)
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	pet := c.character.Pets[index]
	c.pets.register(pet.ID)
	var packets [][]byte
	if fresh {
		packets = append(packets, pet.RecruitPacket(next.ID, s.petTemplate(pet.ID)))
	}
	packets = append(packets, s.petListPacket(c.character, c.pets), petNamePacket(next.ID, c.pets.slot(pet.ID), pet.Name), protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(next.ActivePet))
	packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items)...)
	packets = append(packets, s.World.Sync(c.character, c.view, false)...)
	s.broadcastWorld(c, petMapPacket(next.ID, next.ActivePet, pet.Name))
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, "Companion recruited, selected for battle, and saved.")
}
