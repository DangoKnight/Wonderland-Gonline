package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

// potentialPillCommand handles aLogin FUN_001cce98. Caller holds worldMu;
// ownership, limits and consumption are checked against fresh SQL state.
func (s *Server) potentialPillCommand(ctx context.Context, c *Session, p []byte) error {
	return s.potentialPillWithRoll(ctx, c, p, func() (int, error) {
		roll, err := rand.Int(rand.Reader, big.NewInt(game.PotentialChanceScale))
		if err != nil {
			return 0, err
		}
		return int(roll.Int64()), nil
	})
}

func (s *Server) potentialPillWithRoll(ctx context.Context, c *Session, p []byte, roll func() (int, error)) error {
	if len(p) != protocol.PotentialPillRequestBytes {
		return protocol.ErrMalformed
	}
	if !gmIdle(c) {
		return nil
	}
	bagSlot, target := p[2], p[3]
	reply := []byte{protocol.CommandInventory, protocol.InventoryPotentialPillResult, protocol.PotentialPillRejected, target, 0}
	if bagSlot == 0 || bagSlot > game.BagSize || target > game.MaxPets {
		return c.send(reply)
	}
	var petID uint32
	if target != protocol.PotentialPillPlayerTarget {
		if c.pets == nil {
			return c.send(reply)
		}
		i := s.clientPet(c, target)
		if i < 0 {
			return c.send(reply)
		}
		petID = c.character.Pets[i].ID
		if petID == 0 {
			return c.send(reply)
		}
	}
	expectedItem := c.character.Bag[bagSlot-1].ID
	var unlocks [][]byte
	var calculation []any
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		item := next.Bag[bagSlot-1]
		if item.Empty() || item.Locked || item.ID != expectedItem {
			return game.ErrPotentialPill
		}
		if _, ok := s.Assets.Items[item.ID]; !ok {
			return game.ErrPotentialPill
		}
		potential := &next.Potential
		petIndex := -1
		if petID != 0 {
			petIndex = slices.IndexFunc(next.Pets, func(p game.Pet) bool { return p.ID == petID })
			if petIndex < 0 {
				return game.ErrPotentialPill
			}
			potential = &next.Pets[petIndex].Potential
		}
		chance, err := game.PotentialSuccessPercent(item.ID, *potential)
		if err != nil {
			return err
		}
		outcome := 0
		if chance < game.PotentialChanceScale {
			outcome, err = roll()
			if err != nil {
				return err
			}
		}
		level, err := game.NextPotential(item.ID, *potential, outcome)
		if err != nil {
			return err
		}
		var loggedRoll any
		if chance < game.PotentialChanceScale {
			loggedRoll = outcome
		}
		calculation = []any{
			"session", c.info.ID, "character", next.ID,
			"bag_slot", bagSlot, "item_id", item.ID, "target_slot", target, "pet_id", petID,
			"potential_before", *potential, "attempted_potential", *potential + 1,
			"success_chance_percent", chance, "random_roll_used", loggedRoll != nil,
			"roll", loggedRoll, "roll_max_exclusive", game.PotentialChanceScale,
			"comparison", "roll < success_chance_percent", "success", level > *potential,
			"potential_after", level, "bonus_before", game.PotentialBonus(*potential),
			"bonus_after", game.PotentialBonus(level),
			"bonus_delta_per_attribute", int(game.PotentialBonus(level)) - int(game.PotentialBonus(*potential)),
		}
		if err := next.Bag.Remove(bagSlot, game.PotentialPillQuantity); err != nil {
			return err
		}
		before := next.Clone()
		oldPotential := *potential
		*potential = level
		if petIndex >= 0 {
			next.Pets[petIndex].Normalize(s.Assets.Items, false)
		} else {
			next.RecalculateVitals(s.Assets.Items)
			if level < oldPotential && next.ForgetUnqualifiedSkills() {
				snapshot := skillSnapshotWithRemovals(before, *next)
				packet, err := s.nativeStatsSnapshot(snapshot).BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
				if err != nil {
					return err
				}
				unlocks = [][]byte{packet, {protocol.CommandCharacterState, protocol.CharacterStateRefresh}}
			} else {
				unlocks = s.unlockQualifiedSkills(next, false)
			}
		}
		reply[4] = byte(level)
		return nil
	}, c.character.Clone())
	if err != nil {
		if calculation != nil {
			s.Log.Warn("potential pill attempt not committed", append(calculation, "committed", false, "error", err)...)
		}
		if errors.Is(err, game.ErrItemLocked) || errors.Is(err, game.ErrPotentialPill) || errors.Is(err, game.ErrPotentialMaximum) || errors.Is(err, game.ErrPotentialGoldenMinimum) {
			return s.sendAll(c, [][]byte{reply, serverwire.SystemLine(err.Error())})
		}
		return err
	}
	s.Log.Info("potential pill attempt", append(calculation, "committed", true)...)
	s.adoptSavedCharacter(c, next)
	reply[2] = protocol.PotentialPillProcessed
	// The result drives the native dialog's potential animation and sound. It is
	// AC23:213, not an echo of AC23:126. Publish only after SQL commits.
	packets := [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, bagSlot, game.PotentialPillQuantity}, reply}
	if petID == 0 {
		packets = append(packets, unlocks...)
		packets = append(packets, c.character.StatPackets(s.Assets.Items)...)
	} else {
		i := slices.IndexFunc(c.character.Pets, func(p game.Pet) bool { return p.ID == petID })
		packets = append(packets, c.character.Pets[i].ProgressionPackets(target, s.Assets.Items)...)
	}
	return s.sendAll(c, packets)
}

func (s *Server) rebirthJobCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 3 {
		return protocol.ErrMalformed
	}
	job := p[2]
	if job < game.JobKiller || job > game.JobSeer {
		return protocol.ErrMalformed
	}
	var cape uint16
	for _, class := range s.Assets.RebornClasses {
		if class.Enabled && class.Job == job {
			cape = class.CapeID
			break
		}
	}
	if cape == 0 {
		return nil
	}
	if c.event != nil || c.storm || c.beach != nil {
		return nil
	}
	switch p[1] {
	case protocol.RebirthJobConfirm:
		if len(p) != 3 {
			return protocol.ErrMalformed
		}
		if c.view == nil || !c.view.RebornFormOpen {
			return nil
		}
		return s.performReborn(ctx, c, job, false)
	case protocol.RebirthJobChange:
		if len(p) != 5 {
			return protocol.ErrMalformed
		}
		return s.changeRebornJob(ctx, c, job, cape, p[3], p[4])
	}
	return ErrUnsupported
}

func (s *Server) changeRebornJob(ctx context.Context, c *Session, job byte, cape uint16, mirrorSlot, capeSlot byte) error {
	if mirrorSlot < 1 || mirrorSlot > game.BagSize || capeSlot < 1 || capeSlot > game.BagSize || mirrorSlot == capeSlot {
		return nil
	}
	var packets [][]byte
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		mirror, coat := next.Bag[mirrorSlot-1], next.Bag[capeSlot-1]
		if !next.Reborn || next.Job == job || mirror.ID != game.TransferMirrorItemID || mirror.Empty() || mirror.Locked || coat.ID != cape || coat.Empty() || coat.Locked {
			return fmt.Errorf("job change requires a Transfer Mirror and the selected job's cape")
		}
		if err := next.Bag.Remove(mirrorSlot, 1); err != nil {
			return err
		}
		next.Job, next.RebornJob = job, job
		packets = s.unlockQualifiedSkills(next, true)
		next.Refill(s.Assets.Items)
		return nil
	}, *c.character)
	if err != nil {
		return s.chatFeedback(c, "Job change failed: "+err.Error())
	}
	s.adoptSavedCharacter(c, next)
	packets = append([][]byte{{protocol.CommandInventory, protocol.InventoryRemove, mirrorSlot, 1}, next.ExpPacket()}, packets...)
	packets = append(packets, next.StatPackets(s.Assets.Items)...)
	self, err := next.AppearancePacket(false)
	if err != nil {
		return err
	}
	peer, err := next.AppearancePacket(true)
	if err != nil {
		return err
	}
	s.broadcastWorld(c, peer)
	packets = append(packets, self)
	return s.sendAll(c, packets)
}
