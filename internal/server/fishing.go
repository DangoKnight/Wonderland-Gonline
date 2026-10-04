package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

type fishingRun struct {
	Map, X, Y uint16
	Slot      byte
	Rod       uint16
	NextAt    time.Time
	Legacy    bool // AC90 clients receive the source catch receipt after commit.
}

func fishingSkill(c *game.Character, f assets.FishingRules) *game.LearnedSkill {
	var best *game.LearnedSkill
	for i := range c.Skills {
		sk := &c.Skills[i]
		for _, id := range f.Skills {
			if sk.ID == id && sk.Grade > 0 && (best == nil || sk.Grade > best.Grade) {
				best = sk
			}
		}
	}
	return best
}
func (s *Server) fishingAvailable(c *Session) bool {
	return c.character != nil && c.ready && gmIdle(c) && c.stall == nil && c.arcade == nil && c.party == nil && c.character.HP > 0 && c.character.ActiveVehicle == 0 && c.character.ActiveMount == 0 && c.tentOwner == 0
}
func (s *Server) startFishing(ctx context.Context, c *Session, slot byte) error {
	f := s.Assets.Fishing
	if !f.Enabled {
		return c.send(headBanner("Fishing is not configured."))
	}
	if !s.fishingAvailable(c) {
		return c.send(headBanner("Leave your team and finish active interactions before fishing."))
	}
	terrain, ok := s.Assets.Terrains[c.character.Map]
	if !ok || !f.AllowsMap(c.character.Map) || !terrain.FishingShore(c.character.X, c.character.Y) {
		return c.send(headBanner("Stand near water in a configured fishing area."))
	}
	if slot == 0 {
		var grade byte
		for i, item := range c.character.Bag {
			rod, ok := f.Rod(item.ID)
			if ok && !item.Empty() && !item.Locked && rod.MaxGrade > grade {
				slot = byte(i + 1)
				grade = rod.MaxGrade
			}
		}
	}
	if slot == 0 || int(slot) > len(c.character.Bag) {
		return c.send(headBanner("You need a fishing rod."))
	}
	item := c.character.Bag[slot-1]
	if _, ok := f.Rod(item.ID); !ok || item.Empty() || item.Locked {
		return c.send(headBanner("Select an available fishing rod."))
	}
	_, due, err := s.Store.MutateFishing(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, slot, item.ID, time.Now(), time.Duration(f.IntervalSeconds)*time.Second, false, nil, *c.character)
	if err != nil {
		return err
	}
	c.gathering = nil
	c.fishing = &fishingRun{Map: c.character.Map, X: c.character.X, Y: c.character.Y, Slot: slot, Rod: item.ID, NextAt: due}
	return nil
}
func (s *Server) catchFishing(ctx context.Context, c *Session, now time.Time) error {
	run := c.fishing
	if run == nil {
		return nil
	}
	if !s.fishingAvailable(c) || c.character.Map != run.Map || c.character.X != run.X || c.character.Y != run.Y {
		c.fishing = nil
		return nil
	}

	if run.Slot == 0 || int(run.Slot) > len(c.character.Bag) {
		c.fishing = nil
		return nil
	}
	item := c.character.Bag[run.Slot-1]
	if item.Empty() || item.ID != run.Rod || item.Locked {
		c.fishing = nil
		return nil
	}
	if now.Before(run.NextAt) {
		return nil
	}
	f := s.Assets.Fishing
	rod, ok := f.Rod(run.Rod)
	if !f.Enabled || !ok {
		c.fishing = nil
		return nil
	}
	var adds []game.Addition
	var packets [][]byte
	var reward assets.FishingReward
	var discarded bool
	next, due, err := s.Store.MutateFishing(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, run.Slot, run.Rod, now, time.Duration(f.IntervalSeconds)*time.Second, true, func(next *game.Character) error {
		sk := fishingSkill(next, f)
		var grade byte
		if sk != nil {
			grade = sk.Grade
		}
		var total uint64
		for _, r := range f.Rewards {
			total += f.Weight(r, rod, grade)
		}
		if total == 0 {
			return fmt.Errorf("empty fishing reward pool")
		}
		reward, _ = f.Select(rod, grade, rand.Uint64N(total))
		def, known := s.Assets.Items[reward.ItemID]
		if !known {
			return game.ErrInvalidItem
		}
		var err error
		adds, err = next.Bag.Grant(game.Item{ID: reward.ItemID}, 1, def.StackLimit())
		if errors.Is(err, game.ErrInventoryFull) {
			discarded = true
		} else if err != nil {
			return err
		}
		// Full bags discard the catch, but still earn fishing proficiency. Fishing
		// never learns the quest skill automatically or uses combat EXP thresholds.
		if sk != nil && int(sk.Grade) <= len(f.CatchRequirements) {
			required := f.CatchRequirements[sk.Grade-1]
			if sk.EXP < ^uint32(0) {
				sk.EXP++
			}
			if sk.EXP >= required {
				sk.EXP -= required
				sk.Grade++
				packets = append(packets, protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, game.StatSkillGrade, protocol.StatsValueAbsolute}.U32(uint32(sk.Grade)).U32(uint32(sk.ID)))
			}
			var prof uint32
			if int(sk.Grade) <= len(f.CatchRequirements) {
				prof = uint32(min(uint64(game.SkillProficiencyScale), uint64(sk.EXP)*game.SkillProficiencyScale/uint64(f.CatchRequirements[sk.Grade-1])))
			}
			packets = append(packets, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateSkillProficiency}.U32(uint32(sk.ID)).U16(uint16(prof)))
		}
		return nil
	}, *c.character)
	if errors.Is(err, store.ErrFishingNotDue) {
		if !due.IsZero() {
			run.NextAt = due
		}
		return nil
	}
	if err != nil {
		if errors.Is(err, game.ErrInvalidItem) || errors.Is(err, game.ErrItemLocked) {
			c.fishing = nil
		}
		// A failed SQL transaction consumes no deadline; retry on maintenance.
		return err
	}
	s.adoptSavedCharacter(c, next)
	run.NextAt = due
	if len(adds) > 0 {
		s.sendOrClose(c, c.character.Bag.AdditionPacket(adds))
	}

	if run.Legacy {
		count := byte(1)
		if discarded {
			count = 0
		}
		s.sendOrClose(c, protocol.Builder{protocol.CommandFishing, protocol.FishingCatch}.U16(reward.ItemID).U8(count))
	}
	for _, p := range packets {
		s.sendOrClose(c, p)
	}
	if discarded {
		s.sendOrClose(c, headBanner("Your bag is full. The catch was discarded; fishing proficiency was retained."))
	}
	return nil
}
func (s *Server) fishingCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.FishingStart:
		if err := s.startFishing(ctx, c, 0); err != nil {
			return err
		}
		active := byte(protocol.NativeInactive)
		if c.fishing != nil {
			c.fishing.Legacy = true
			active = protocol.NativeActive
		}
		return c.send([]byte{protocol.CommandFishing, protocol.FishingStart, active})
	case protocol.FishingCatch:
		return s.catchFishing(ctx, c, time.Now())
	case protocol.FishingStop:
		c.fishing = nil
		return c.send([]byte{protocol.CommandFishing, protocol.FishingStop, protocol.NativeInactive})
	}
	return ErrUnsupported
}
func (s *Server) tickFishing(ctx context.Context, now time.Time) {
	for _, c := range s.world {
		if c.fishing != nil {
			if err := s.catchFishing(ctx, c, now); err != nil {
				s.Log.Error("fishing catch failed", "character", c.character.ID, "error", err)
			}
		}
	}
}
