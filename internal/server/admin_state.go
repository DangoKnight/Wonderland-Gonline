package server

import (
	"errors"
	"fmt"
	"wonderland-go/internal/game"
)

// The caller holds worldMu and a catalog lock, excluding initial map snapshots.
func (s *Server) adminCharacterLoading(id uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.sessions {
		if c.info.CharacterID == id && s.friendSessions[id] != c {
			return true
		}
	}
	return false
}

func (s *Server) validateAdminState(c game.Character) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Element < game.Earth || c.Element > game.Wind || c.Gold > game.MaxGold || len(c.Pets) > game.MaxPets {
		return errors.New("invalid element, gold or pet count")
	}
	validateItems := func(items []game.Item, equipped bool) error {
		for _, item := range items {
			if item.ID == 0 && item.Count == 0 {
				continue
			}
			limit, known := s.stackLimit(item.ID)
			if !known || item.Count == 0 || item.Count > limit || (equipped && item.Count != 1) {
				return fmt.Errorf("invalid item or stack %d", item.ID)
			}
		}
		return nil
	}
	for _, group := range []struct {
		items    []game.Item
		equipped bool
	}{{c.Bag[:], false}, {c.Storage[:], false}, {c.Equipment[:], true}} {
		if err := validateItems(group.items, group.equipped); err != nil {
			return err
		}
	}
	for _, inventory := range []game.Inventory{c.Bag, c.Storage} {
		if _, err := inventory.Occupancy(s.Assets.Items); err != nil {
			return fmt.Errorf("invalid inventory footprint: %w", err)
		}
	}
	seenSkills := map[uint16]bool{}
	for _, skill := range c.Skills {
		if _, known := s.Assets.Skills[skill.ID]; !known || seenSkills[skill.ID] || skill.Grade < game.MinSkillGrade || skill.Grade > game.MaxSkillGrade {
			return errors.New("invalid or duplicate skill")
		}
		seenSkills[skill.ID] = true
	}
	seenPets := map[uint32]bool{}
	active := c.ActivePet == 0
	mount := c.ActiveMount == 0
	for groupIndex, pets := range [][]game.Pet{c.Pets, c.ReservePets, c.HotelPets} {
		for _, pet := range pets {
			if pet.ID == 0 || pet.Level < 1 || pet.Level > game.MaxLevel || pet.HP < 0 || pet.HP > pet.MaxHP || pet.SP < 0 || pet.SP > pet.MaxSP || seenPets[game.BroadcastID(pet.ID)] {
				return errors.New("invalid or duplicate pet")
			}
			seenPets[game.BroadcastID(pet.ID)] = true
			if err := validateItems(pet.Equipment[:], true); err != nil {
				return err
			}
			if groupIndex == 0 {
				active = active || game.SamePet(c.ActivePet, pet.ID)
				mount = mount || game.SamePet(c.ActiveMount, pet.ID)
			}
		}
	}
	if !active || !mount {
		return errors.New("active pet or mount is not in the party")
	}
	if c.RecordPoint != nil {
		if _, ok := s.Assets.Maps[c.RecordPoint.Map]; !ok {
			return errors.New("unknown record map")
		}
	}
	for key, expiry := range c.ChestRespawns {
		if key == 0 || expiry.IsZero() {
			return errors.New("invalid chest cooldown")
		}
	}
	if c.ActiveVehicle != 0 && (c.VehicleSlot < 1 || int(c.VehicleSlot) > len(c.Bag) || c.Bag[c.VehicleSlot-1].ID != c.ActiveVehicle) {
		return errors.New("vehicle is not in the selected bag slot")
	}
	return nil
}

func (s *Server) AdminKickAll() {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.mu.Lock()
	targets := make([]*Session, 0, len(s.sessions))
	for _, c := range s.sessions {
		targets = append(targets, c)
	}
	s.mu.Unlock()
	for _, c := range targets {
		s.gmKick(c, "Server maintenance.")
	}
}
