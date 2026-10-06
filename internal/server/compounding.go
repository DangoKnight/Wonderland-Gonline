package server

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

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
