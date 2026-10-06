package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	gmGodAttribute = 999
	gmTentItem     = 34001
)

func (s *Server) gmPlayerEdit(ctx context.Context, c *Session, command string, args []string) error {
	next := c.character.Clone()
	var packets [][]byte
	if command == "allskills" || command == "maxskills" {
		if len(args) > 1 {
			return s.chatFeedback(c, "Usage: /allskills [grade]")
		}
		grade := uint64(game.MinSkillGrade)
		if len(args) == 1 {
			n, err := strconv.ParseUint(args[0], 10, 8)
			if err != nil {
				return s.chatFeedback(c, "Grade must be a number from 1 to 10.")
			}
			grade = min(max(n, game.MinSkillGrade), game.MaxSkillGrade)
		}
		for _, id := range next.GMElementSkillIDs() {
			wireID := id
			if id == game.StarterStunt(next.Body, next.Head) {
				wireID = game.StarterStuntClientID
			}
			if _, ok := s.Assets.Skills[wireID]; !ok {
				return s.chatFeedback(c, fmt.Sprintf("Skill %d is missing from the assets catalog; no skills changed.", wireID))
			}
			next.SetSkillGrade(id, byte(grade))
		}
	} else {
		if len(args) != 0 {
			return s.chatFeedback(c, "Usage: /god")
		}
		next.Base = game.Attributes{Strength: gmGodAttribute, Constitution: gmGodAttribute, Intelligence: gmGodAttribute, Wisdom: gmGodAttribute, Agility: gmGodAttribute}
		next.Refill(s.Assets.Items)
		if index := gmSelectedPet(&next); index >= 0 {
			pet := &next.Pets[index]
			pet.Base = next.Base
			// Keep durable vitals consistent with native roster normalization.
			pet.Normalize(s.Assets.Items, true)
			packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items)...)
			packets = append(packets, s.petListPacket(&next, c.pets))
		}
		packets = append(packets, next.StatPackets(s.Assets.Items)...)
	}
	snapshot, err := s.nativeStatsSnapshot(next).BaseStatsPacket(func(id uint16) (uint16, bool) { sk, ok := s.Assets.Skills[id]; return sk.TableOrder, ok })
	if err != nil {
		return err
	}
	packets = append(packets, snapshot, []byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	if err = s.commit(ctx, c, next); err != nil {
		return err
	}
	if err = s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, "GM character update saved.")
}

func (s *Server) gmInventory(ctx context.Context, c *Session, command string, args []string) error {
	switch command {
	case "clearinv":
		if len(args) != 0 {
			return s.chatFeedback(c, "Usage: /clearinv")
		}
		next := c.character.Clone()
		var packets [][]byte
		for i, item := range next.Bag {
			if !item.Empty() {
				packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, byte(i + 1), item.Count})
			}
		}
		next.Bag = game.Inventory{}
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		if err := s.sendAll(c, packets); err != nil {
			return err
		}
		return s.chatFeedback(c, "Inventory cleared; equipment and storage preserved.")
	case "tent":
		if len(args) != 0 {
			return s.chatFeedback(c, "Usage: /tent")
		}
		for _, item := range c.character.Bag {
			if item.ID == gmTentItem && !item.Empty() {
				return s.chatFeedback(c, "You already have a Tent in your inventory.")
			}
		}
		if !s.hasItem(gmTentItem) {
			return s.chatFeedback(c, "Tent item is missing from the assets catalog.")
		}
		return s.giveItem(ctx, c, []string{"/item", strconv.Itoa(gmTentItem), "1"})
	case "buy":
		if len(args) == 0 {
			return s.chatFeedback(c, "Usage: /buy <item ID or name> [count]")
		}
		quantity := uint64(1)
		if len(args) > 1 {
			if n, err := strconv.ParseUint(args[len(args)-1], 10, 8); err == nil {
				quantity = n
				args = args[:len(args)-1]
			}
		}
		if quantity == 0 {
			return s.chatFeedback(c, "Quantity must be positive.")
		}
		query := strings.Join(args, " ")
		id, idErr := strconv.ParseUint(query, 10, 16)
		for _, entry := range s.mallEntries(false) {
			if (idErr == nil && uint16(id) == entry.ID) || (idErr != nil && strings.Contains(strings.ToLower(entry.Name), strings.ToLower(query))) {
				return s.purchaseMall(ctx, c, []mallCartRow{{item: entry.ID, category: mallCategory(entry), order: mallOrder(entry), quantity: byte(quantity)}}, false, true)
			}
		}
		if idErr == nil && id > 0 && s.hasItem(uint16(id)) {
			return s.giveItem(ctx, c, []string{"/item", query, strconv.FormatUint(quantity, 10)})
		}
		return s.chatFeedback(c, "Item not found in the mall or assets catalog.")
	}
	return nil
}
