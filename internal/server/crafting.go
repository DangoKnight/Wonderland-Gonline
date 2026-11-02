package server

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

// AC40 retains its legacy two-slot signature and echo, using the same new
// rank/base engine as AC23. Catastrophic failure delivers its junk output.
func (s *Server) alchemyCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.AlchemyRequestBytes {
		return protocol.ErrMalformed
	}
	if !gmIdle(c) || c.stall != nil {
		return nil
	}
	execution, err := s.executeCompounding(ctx, c, p[2:], true)
	if err != nil {
		if compoundingRejection(err) {
			return c.send(protocol.Builder{protocol.CommandAlchemy, p[1], protocol.AlchemyFailed}.U16(0))
		}
		return err
	}
	packets := [][]byte{
		{protocol.CommandInventory, protocol.InventoryRemove, p[2], 1},
		{protocol.CommandInventory, protocol.InventoryRemove, p[3], 1},
		c.character.Bag.AdditionPacket(execution.Additions),
		protocol.Builder{protocol.CommandAlchemy, p[1], protocol.AlchemySucceeded}.U16(execution.Outcome.ItemID)}
	return s.sendAll(c, append(packets, execution.SkillPackets...))
}

// AC23:14/87/101 select Primary/Junior/Superior: count followed by ordered, distinct bag anchors. Two to five total
// inputs are accepted; books are excluded from the minimum material count.
func (s *Server) compoundCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < protocol.CompoundRequestHeaderBytes {
		return protocol.ErrMalformed
	}
	count := int(p[2])
	if count < protocol.CompoundIngredientSlots || count > protocol.CompoundMaximumIngredients || len(p) != protocol.CompoundRequestHeaderBytes+count {
		return protocol.ErrMalformed
	}
	if !gmIdle(c) || c.stall != nil {
		return nil
	}
	slots := p[protocol.CompoundRequestHeaderBytes:]
	tier := game.AlchemyPrimary
	switch p[1] {
	case protocol.InventoryCompoundJunior:
		tier = game.AlchemyJunior
	case protocol.InventoryCompoundSuperior:
		tier = game.AlchemySuperior
	}
	execution, err := s.executeCompounding(ctx, c, slots, false, tier)
	if err != nil {
		if compoundingRejection(err) {
			return c.send(serverwire.SystemLine(err.Error()))
		}
		return err
	}
	result := execution.Outcome.ItemID
	packets := make([][]byte, 0, len(slots)+2+len(execution.SkillPackets))
	for _, slot := range slots {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, 1})
	}
	packets = append(packets,
		protocol.Builder{protocol.CommandInventory, protocol.InventoryCompoundResult, execution.Target}.U16(result).U8(1).Bytes(make([]byte, protocol.CompoundResultReservedBytes)),
		protocol.Builder{protocol.CommandInventory, protocol.InventoryCompoundSuccess}.U16(result).U8(1).U8(execution.Target))
	packets = append(packets, execution.SkillPackets...)
	return s.sendAll(c, packets)
}

type compoundExecution struct {
	Outcome      game.AlchemyOutcome
	Target       byte
	Additions    []game.Addition
	SkillPackets [][]byte
}

func secureAlchemyRoll(n int) (int, error) {
	r, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(r.Int64()), nil
}
func compoundingRejection(err error) bool {
	return errors.Is(err, game.ErrAlchemyIngredients) || errors.Is(err, game.ErrAlchemyMaterial) || errors.Is(err, game.ErrAlchemyResult) || errors.Is(err, game.ErrInvalidItem) || errors.Is(err, game.ErrItemLocked) || errors.Is(err, game.ErrInventoryFull)
}

// Caller holds worldMu. This is shared by native AC23 and compatibility AC40;
// SQL ownership/resources/skills are authoritative for every roll and delivery.
func (s *Server) executeCompounding(ctx context.Context, c *Session, slots []byte, merge bool, requested ...game.AlchemyTier) (compoundExecution, error) {
	execution := compoundExecution{}
	if len(slots) < game.AlchemyMinimumMaterials || len(slots) > game.AlchemyMaximumIngredients {
		return execution, game.ErrAlchemyIngredients
	}
	expected := make([]game.Item, len(slots))
	seen := map[byte]bool{}
	for i, slot := range slots {
		if slot < 1 || slot > game.BagSize || seen[slot] {
			return execution, game.ErrInvalidItem
		}
		seen[slot] = true
		expected[i] = c.character.Bag[slot-1]
		if expected[i].Empty() || expected[i].Locked || c.character.ActiveVehicle != 0 && slot == c.character.VehicleSlot {
			return execution, game.ErrInvalidItem
		}
	}
	if s.alchemyIndex == nil || s.alchemyAssets != s.Assets {
		s.alchemyIndex = s.Assets.CompoundingCatalog()
		s.alchemyAssets = s.Assets
	}
	roll := s.alchemyRandom
	if roll == nil {
		roll = secureAlchemyRoll
	}
	var skillID uint16
	var beforeEXP, afterEXP uint32
	var beforeGrade, afterGrade byte
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		ids := make([]uint16, len(slots))
		for i, slot := range slots {
			it := next.Bag[slot-1]
			if it != expected[i] || it.Empty() || it.Locked || next.ActiveVehicle != 0 && slot == next.VehicleSlot {
				return game.ErrInvalidItem
			}
			ids[i] = it.ID
		}
		tier, level, skill := next.AlchemySkill()
		if len(requested) != 0 {
			tier, level, skill = requested[0], 0, 0
			wanted := uint16(game.AlchemyPrimarySkill)
			switch tier {
			case game.AlchemyJunior:
				wanted = game.AlchemyJuniorSkill
			case game.AlchemySuperior:
				wanted = game.AlchemySuperiorSkill
			}
			for _, sk := range next.Skills {
				if sk.ID == wanted && sk.Grade > 0 {
					level, skill = min(sk.Grade, game.AlchemySkillMaximum), sk.ID
				}
			}
			if tier != game.AlchemyPrimary && skill == 0 {
				return game.ErrAlchemyIngredients
			}
		}
		skillID = skill
		for _, sk := range next.Skills {
			if sk.ID == skill {
				beforeEXP, beforeGrade = sk.EXP, sk.Grade
			}
		}
		outcome, err := s.alchemyIndex.Compound(ids, tier, level, roll)
		if err != nil {
			return err
		}
		if _, ok := s.Assets.Items[outcome.ItemID]; !ok {
			return game.ErrAlchemyResult
		}
		execution.Outcome = outcome
		if merge {
			for _, slot := range slots {
				if err := next.Bag.Remove(slot, 1); err != nil {
					return err
				}
			}
			limit, ok := s.stackLimit(outcome.ItemID)
			if !ok {
				return game.ErrAlchemyResult
			}
			execution.Additions, err = next.Bag.Grant(game.Item{ID: outcome.ItemID}, 1, limit, s.Assets.Items)
		} else {
			execution.Target, err = next.Bag.CompoundSlots(slots, outcome.ItemID, s.Assets.Items)
		}
		if err != nil {
			return err
		}
		execution.SkillPackets = next.AddSkillEXP(skill, game.AlchemySkillEXPGain)
		for _, sk := range next.Skills {
			if sk.ID == skill {
				afterEXP, afterGrade = sk.EXP, sk.Grade
			}
		}
		return nil
	}, c.character.Clone())
	if err != nil {
		return compoundExecution{}, err
	}
	s.adoptSavedCharacter(c, next)
	o := execution.Outcome
	s.Log.Debug("compounding result", "character", next.ID, "slots", slots, "tier", o.Tier, "skill_level", o.Level, "skill_id", skillID, "skill_exp_before", beforeEXP, "skill_exp_after", afterEXP, "skill_grade_before", beforeGrade, "skill_grade_after", afterGrade, "base_rank", o.BaseRank, "primary_base", o.PrimaryBase, "required_bases", o.RequiredBases, "book_bonus", o.BookBonus, "rolled_delta", o.Delta, "rank_ceiling", o.RankCeiling, "result_rank", o.ResultRank, "result", o.ItemID, "secondary_dropped", o.SecondaryDropped, "catastrophic", o.Catastrophic)
	return execution, nil
}

func forgeRoll() (bool, error) {
	var value [1]byte
	_, err := rand.Read(value[:])
	return value[0]&1 == 0, err
}

// forgeReject clears the native pending selection before its explanation.
func forgeReject(c *Session, result byte, message string) error {
	if err := c.send([]byte{protocol.CommandMall, protocol.MallForgeResult, result}); err != nil {
		return err
	}
	return c.send(serverwire.HeadBanner(message))
}
func (s *Server) forgeCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.MallForgeRequestBytes {
		return protocol.ErrMalformed
	}
	return s.forgeItem(ctx, c, p[2], forgeRoll)
}

// forgeItem ports MallForgingManager. Caller holds worldMu; point payment and
// metadata changes use the same authoritative transaction as mall purchases.
func (s *Server) forgeItem(ctx context.Context, c *Session, slot byte, roll func() (bool, error)) error {
	reject := func(message string) error { return forgeReject(c, protocol.MallForgeRejected, message) }
	if c.event != nil || c.storm || c.beach != nil {
		return reject("Forging is unavailable right now.")
	}
	if slot < 1 || slot > game.BagSize {
		return reject("Select equipment in your inventory.")
	}
	source := c.character.Bag[slot-1]
	if source.Empty() || source.Locked || source.Count != 1 {
		return reject("Select one equipment item in your inventory.")
	}
	if c.character.ActiveVehicle != 0 && slot == c.character.VehicleSlot {
		return reject("Land the vehicle before forging it.")
	}
	definition, known := s.Assets.Items[source.ID]
	if !known {
		return reject("Equipment data is unavailable.")
	}
	if upgrade, ok := s.Assets.Forging.Upgrades[source.ID]; ok {
		if upgrade.Next == 0 {
			return reject("This equipment is already at its maximum upgrade.")
		}
		target, known := s.Assets.Items[upgrade.Next]
		if !known || target.EquipSlot < 1 || int(target.EquipSlot) > len(c.character.Equipment) || target.EquipSlot != definition.EquipSlot {
			return reject("Upgrade data is unavailable. Equipment and scrolls retained.")
		}
		next := c.character.Clone()
		needed := int(upgrade.Scrolls)
		var costs []game.Addition
		for index, item := range next.Bag {
			if needed == 0 {
				break
			}
			if item.ID != game.StrongScrollItemID || item.Empty() || (next.ActiveVehicle != 0 && next.VehicleSlot == byte(index+1)) {
				continue
			}
			take := byte(min(needed, int(item.Count)))
			if err := next.Bag.Remove(byte(index+1), take); err != nil {
				return err
			}
			costs = append(costs, game.Addition{Slot: byte(index + 1), Count: take})
			needed -= int(take)
		}
		if needed != 0 {
			return forgeReject(c, protocol.MallForgeInsufficientScrolls, fmt.Sprintf("This upgrade requires %d Strong Scroll(s).", upgrade.Scrolls))
		}
		next.Bag[slot-1] = game.Item{}
		if !next.Bag.CanPlace(slot, game.Item{ID: upgrade.Next, Count: 1}, s.Assets.Items) {
			return reject("The upgraded item does not fit. Equipment and scrolls retained.")
		}
		next.Bag[slot-1] = game.Item{ID: upgrade.Next, Count: 1, Damage: source.Damage}
		if err := s.commit(ctx, c, next); err != nil {
			return err
		}
		var packets [][]byte
		for _, cost := range costs {
			packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, cost.Slot, cost.Count})
		}
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, next.Bag.AdditionPacket([]game.Addition{{Slot: slot, Count: 1}}), []byte{protocol.CommandMall, protocol.MallForgeResult, protocol.MallForgeUpgradeSucceeded})
		return s.sendAll(c, packets)
	}
	if !s.Assets.Forging.PointItems[source.ID] || !definition.CanPointForge() {
		return reject("This equipment cannot be forged.")
	}
	if source.Forge() >= game.MaxForgeProgress {
		return reject("This equipment is already at its maximum forging progress.")
	}
	var success bool
	next, balances, err := s.Store.PurchaseMall(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, false, game.PointForgeCost, func(character *game.Character) error {
		if err := game.PreserveItemLocks(*c.character, character); err != nil {
			return err
		}
		if character.Bag[slot-1] != source {
			return game.ErrInvalidItem
		}
		var err error
		success, err = roll()
		if err != nil {
			return err
		}
		if success {
			character.Bag[slot-1].SetForge(source.Forge() + 1)
		}
		return nil
	})
	if errors.Is(err, store.ErrMallFunds) {
		if err := forgeReject(c, protocol.MallForgeInsufficientPoints, "Forging requires 3 IM Points."); err != nil {
			return err
		}
		return s.sendMallBalances(ctx, c)
	}
	if errors.Is(err, game.ErrInvalidItem) {
		return reject("The selected item changed. Please select it again.")
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	reply := []byte{protocol.CommandMall, protocol.MallForgeResult, protocol.MallForgeRollFailed}
	if success {
		reply = []byte{protocol.CommandMall, protocol.MallForgeResult, protocol.MallForgeProgressSucceeded, slot, next.Bag[slot-1].Forge(), 0}
	}
	return s.sendAll(c, append([][]byte{reply}, serverwire.MallBalancePackets(balances)...))
}

// The chat shortcut shares the same SQL-owned rank/base engine as the UI.
func (s *Server) synthesize(ctx context.Context, c *Session, slots ...byte) error {
	if !gmIdle(c) || c.stall != nil {
		return nil
	}
	execution, err := s.executeCompounding(ctx, c, slots, true)
	if err != nil {
		if compoundingRejection(err) {
			return s.chatFeedback(c, err.Error())
		}
		return err
	}
	packets := [][]byte{}
	for _, slot := range slots {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, slot, 1})
	}
	packets = append(packets, c.character.Bag.AdditionPacket(execution.Additions))
	packets = append(packets, execution.SkillPackets...)
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, fmt.Sprintf("Synthesis produced %s (rank %d).", s.Assets.Items[execution.Outcome.ItemID].Name, execution.Outcome.ResultRank))
}

func isNativeHotbarAssignment(p []byte) bool {
	return len(p) == protocol.HotbarRequestBytes && p[1] == protocol.HotbarAssign
}

// Native AC40:1 is a presentation preference, not synthesis. aLogin installs
// the binding locally before sending FUN_002aa92c. Accept it without emitting
// the incompatible five-byte legacy alchemy result or modifying resources.
// Durable hotbar storage and login replay are pending; acceptance alone must
// not imply that a new skill was learned or an item was acquired.
func (s *Server) hotbarOrAlchemyCommand(ctx context.Context, c *Session, p []byte) error {
	if !isNativeHotbarAssignment(p) {
		return s.alchemyCommand(ctx, c, p)
	}
	kind, page, slot := p[protocol.HotbarKindOffset], p[protocol.HotbarPageOffset], p[protocol.HotbarSlotOffset]
	if (kind != protocol.HotbarItem && kind != protocol.HotbarSkill) || page < 1 || page > protocol.HotbarPages || slot < 1 || slot > protocol.HotbarSlotsPerPage {
		return protocol.ErrMalformed
	}
	s.Log.Debug("native hotbar assignment accepted", "session", c.info.ID, "character", c.character.ID,
		"kind", kind, "id", binary.LittleEndian.Uint16(p[protocol.HotbarIDOffset:]), "page", page, "slot", slot)
	return nil
}
