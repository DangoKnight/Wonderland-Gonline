package server

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

// GM progression changes don't mutate snapshots owned by combat, events,
// loading or trades. Caller holds worldMu and has checked administrator rights.
func (s *Server) gmProgress(ctx context.Context, c *Session, name string, words []string) error {
	if !commandTravelAvailable(c) || c.trade != nil {
		return nil
	}
	next := c.character.Clone()
	var packets [][]byte
	refill := false
	switch name {
	case "level", "lvl":
		if len(words) < 2 {
			return nil
		}
		level, err := strconv.ParseUint(words[1], 10, 8)
		if err != nil {
			return nil
		}
		next.SetLevel(byte(level))
		refill = true
		packets = append(packets, next.ExpPacket())
	case "points", "sp", "statpoint", "statpoints":
		if len(words) < 2 {
			return nil
		}
		points, err := strconv.ParseUint(words[1], 10, 16)
		if err != nil {
			return nil
		}
		next.StatPoints = uint16(min(uint64(next.StatPoints)+points, math.MaxUint16))
		packets = append(packets, protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, game.StatUnallocatedPoints, protocol.StatsValueAbsolute}.U32(uint32(next.StatPoints)).U32(0))
	case "stats", "stat":
		if len(words) < 6 {
			return nil
		}
		var values [5]uint16
		for i := range values {
			v, err := strconv.ParseUint(words[i+1], 10, 16)
			if err != nil {
				return nil
			}
			values[i] = uint16(v)
		}
		next.Base = game.Attributes{Strength: values[0], Constitution: values[1], Intelligence: values[2], Wisdom: values[3], Agility: values[4]}
		refill = true
	case "exp":
		if len(words) < 2 {
			return nil
		}
		exp, err := strconv.ParseInt(words[1], 10, 64)
		if err != nil || exp > math.MaxUint32 {
			return nil
		}
		next.EXP = uint32(max(exp, 0))
		next.Level = next.LevelFromEXP(uint64(next.EXP))
		// Direct assignment grants no points (AC02 sets TotalExp, not AddExp).
		packets = append(packets, next.ExpPacket())
	case "skill":
		if len(words) < 2 {
			return nil
		}
		id, err := strconv.ParseUint(words[1], 10, 16)
		if err != nil || !s.hasSkill(uint16(id)) {
			return nil
		}
		grade := uint64(game.MinSkillGrade)
		if len(words) >= 3 {
			grade, err = strconv.ParseUint(words[2], 10, 8)
			if err != nil {
				return nil
			}
		}
		if grade < game.MinSkillGrade || grade > uint64(game.SkillGradeLimit(uint16(id))) {
			return nil
		}
		packets = next.SetSkillGrade(uint16(id), byte(grade))
	default:
		return nil
	}
	if name == "level" || name == "lvl" || name == "stat" || name == "stats" || name == "exp" {
		if refill {
			next.Refill(s.Assets.Items)
		} else {
			full := next.Combat(s.Assets.Items)
			next.MaxHP, next.MaxSP = uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
			next.HP, next.SP = min(next.HP, next.MaxHP), min(next.SP, next.MaxSP)
		}
		packets = append(packets, next.StatPackets(s.Assets.Items)...)
	}
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	return s.sendAll(c, packets)
}

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
			pet.Normalize(s.Assets.Items, true, s.petTemplate(pet.ID))
			packets = append(packets, pet.ProgressionPackets(c.pets.slot(pet.ID), s.Assets.Items, s.petTemplate(pet.ID))...)
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

func (s *Server) gmReborn(ctx context.Context, c *Session, command string, args []string) error {
	if len(args) < 1 || len(args) > 2 || len(args) == 2 && !strings.EqualFold(args[1], "force") {
		return s.chatFeedback(c, "Usage: /reborn <Killer|Warrior|Knight|Wit|Priest|Seer|1-6> [force]")
	}
	query := args[0]
	if strings.EqualFold(query, "sage") || strings.EqualFold(query, "mage") {
		query = "Wit"
	}
	var job byte
	for _, class := range s.Assets.RebornClasses {
		if class.Enabled && (strings.EqualFold(class.Name, query) || strconv.Itoa(int(class.Job)) == query) {
			job = class.Job
			break
		}
	}
	if job == game.JobNone {
		return s.chatFeedback(c, "That reborn class/cape is unavailable.")
	}
	return s.performReborn(ctx, c, job, len(args) == 2)
}

func (s *Server) performReborn(ctx context.Context, c *Session, job byte, force bool) error {
	var skillPackets [][]byte
	var marks []uint32
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		if !next.RebornAs(job, force) {
			return fmt.Errorf("rebirth requires level %d and a character that has not reborn", game.RebornMinimumLevel)
		}
		skillPackets = s.unlockQualifiedSkills(next, true)
		next.Refill(s.Assets.Items)
		var err error
		marks, err = s.applyRebornCheckpoints(next, time.Now().UTC())
		return err
	}, *c.character)
	if err != nil {
		return s.chatFeedback(c, "Rebirth failed: "+err.Error())
	}
	s.adoptSavedCharacter(c, next)
	c.view.RebornFormOpen = false
	packets := append([][]byte{next.ExpPacket()}, next.StatPackets(s.Assets.Items)...)
	packets = append(packets, skillPackets...)
	base, err := s.nativeStatsSnapshot(next).BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
	if err != nil {
		return err
	}
	packets = append(packets, base)
	appearance, err := next.AppearancePacket(true)
	if err != nil {
		return err
	}
	s.broadcastWorld(c, appearance)
	self, err := next.AppearancePacket(false)
	if err != nil {
		return err
	}
	packets = append(packets, self)
	effect := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateShapeshift}.U32(next.ID).U16(0)
	s.broadcastWorld(c, effect)
	packets = append(packets, effect)
	for _, mark := range marks {
		packets = append(packets, s.World.QuestUpdate(c.view, mark, next.Quests[mark])...)
	}
	if err = s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, fmt.Sprintf("Rebirth completed: level %d.", next.Level))
}

// gmRestat ports GmManager.RestatPlayer, saving before stat/snapshot replies.
// Caller holds worldMu and has checked the actor's GM privileges.
func (s *Server) gmRestat(ctx context.Context, actor *Session, arguments []string) error {
	arguments = strings.Fields(strings.Join(arguments, " "))
	if len(arguments) > 1 {
		return s.chatFeedback(actor, "Usage: /restat [character ID or name]")
	}
	target := actor
	if len(arguments) == 1 {
		target = s.findOnline(arguments[0])
		if target == nil {
			return s.chatFeedback(actor, "That character is offline or unavailable.")
		}
	}
	if !commandTravelAvailable(actor) || actor.trade != nil || !commandTravelAvailable(target) || target.trade != nil {
		return s.chatFeedback(actor, "Finish active interactions before resetting attributes.")
	}
	next := target.character.Clone()
	refund := next.ResetAttributes()
	next.Refill(s.Assets.Items)
	snapshot, err := s.nativeStatsSnapshot(next).BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
	if err != nil {
		return err
	}
	if err = s.commit(ctx, target, next); err != nil {
		return err
	}
	packets := append(next.StatPackets(s.Assets.Items), snapshot)
	if err = s.sendAll(target, packets); err != nil {
		if target != actor {
			target.conn.Close()
			return s.chatFeedback(actor, "Attribute reset saved; the target must reconnect to refresh stats.")
		}
		return err
	}
	return s.chatFeedback(actor, fmt.Sprintf("Reset attributes for %s; refunded %d points (%d available).", next.Name, refund, next.StatPoints))
}

// gmClearSkills follows GmManager.ClearSkills and SkillManager initialization.
// Caller holds worldMu and has checked the actor's GM authorization.
func (s *Server) gmClearSkills(ctx context.Context, actor *Session, arguments []string) error {
	arguments = strings.Fields(strings.Join(arguments, " "))
	if len(arguments) > 1 {
		return s.chatFeedback(actor, "Usage: /clearskills [character ID or name]")
	}
	target := actor
	if len(arguments) == 1 {
		target = s.findOnline(arguments[0])
		if target == nil {
			return s.chatFeedback(actor, "That character is offline or unavailable.")
		}
	}
	if !commandTravelAvailable(actor) || actor.trade != nil || !commandTravelAvailable(target) || target.trade != nil {
		return s.chatFeedback(actor, "Finish active interactions before resetting skills.")
	}
	next := target.character.Clone()
	next.Skills = game.StarterSkills(next.Body, next.Head, next.Element)
	s.unlockQualifiedSkills(&next, false)

	snapshotCharacter := skillSnapshotWithRemovals(*target.character, next)
	snapshot, err := s.nativeStatsSnapshot(snapshotCharacter).BaseStatsPacket(func(id uint16) (uint16, bool) {
		skill, ok := s.Assets.Skills[id]
		return skill.TableOrder, ok
	})
	if err != nil {
		return err
	}
	if err = s.commit(ctx, target, next); err != nil {
		return err
	}
	if err = s.sendAll(target, [][]byte{snapshot, {protocol.CommandCharacterState, protocol.CharacterStateRefresh}}); err != nil {
		if target != actor {
			target.conn.Close()
			return s.chatFeedback(actor, "Skill reset saved; the target must reconnect to refresh skills.")
		}
		return err
	}
	return s.chatFeedback(actor, fmt.Sprintf("Reset skills for %s to %d starter/qualified skills at grade 1 with zero proficiency.", next.Name, len(next.Skills)))
}

// gmRepair ports GmManager.RepairAllItems. The caller holds worldMu and has
// checked GM authorization. Missing targets must not fall back to repairing self.
func (s *Server) gmRepair(ctx context.Context, actor *Session, arguments []string) error {
	arguments = strings.Fields(strings.Join(arguments, " "))
	if len(arguments) > 1 {
		return s.chatFeedback(actor, "Usage: /repair [character ID or name]")
	}
	target := actor
	if len(arguments) == 1 {
		target = s.findOnline(arguments[0])
		if target == nil {
			return s.chatFeedback(actor, "That character is offline or unavailable.")
		}
	}
	if !commandTravelAvailable(actor) || actor.trade != nil || !commandTravelAvailable(target) || target.trade != nil {
		return s.chatFeedback(actor, "Finish active interactions before repairing items.")
	}
	next := target.character.Clone()
	var additions []game.Addition
	var packets [][]byte
	repaired := 0
	for i, item := range next.Bag {
		if item.Empty() || item.Damage == 0 {
			continue
		}
		next.Bag[i].Damage = 0
		slot := byte(i + 1)
		// AC23:5 adds records; remove the old stack before adding its repair.
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, item.Count})
		additions = append(additions, game.Addition{Slot: slot, Count: item.Count})
		repaired++
	}
	gearChanged := false
	for i, item := range next.Equipment {
		if item.Empty() || item.Damage == 0 {
			continue
		}
		next.Equipment[i].Damage = 0
		gearChanged = true
		repaired++
	}
	if repaired == 0 {
		return s.chatFeedback(actor, fmt.Sprintf("%s has no damaged equipped or inventory items.", next.Name))
	}
	if err := s.commit(ctx, target, next); err != nil {
		return err
	}
	if len(additions) > 0 {
		packets = append(packets, next.Bag.AdditionPacket(additions))
	}
	if gearChanged {
		packets = append(packets, next.EquipmentPacket())
	}
	if err := s.sendAll(target, packets); err != nil {
		if target != actor {
			target.conn.Close()
			return s.chatFeedback(actor, "Repair saved; the target must reconnect to refresh items.")
		}
		return err
	}
	return s.chatFeedback(actor, fmt.Sprintf("Repaired %d equipped/inventory item stacks for %s.", repaired, next.Name))
}
