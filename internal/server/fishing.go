package server

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

const (
	nativeFishingGroundRecordBytes = 15
	nativeFishingGroundSlotLimit   = 255

	nativeFishingRequestBytes = 4
	// FUN_003cfaf0 reads native item memory offset 0x31, after a four-byte prefix.
	nativeFishingPresentationOffset       = 45
	nativeFishingDefaultPresentation byte = 1
	nativeFishingMaxPresentation     byte = 6
)

// Native AC23:53 carries the selected bag slot and a presentation value.
// The sender at 0x2c9ea8 reads client fields 0x20b6/0x20b7. The latter
// chooses fishing animation frames; SQL rod definitions own gameplay limits.
// Older two-byte requests (and the prior zero-word adapter) select a rod automatically.
func fishingStartSlot(p []byte) (byte, error) {
	if len(p) == 2 {
		return 0, nil
	}
	if len(p) != nativeFishingRequestBytes {
		return 0, protocol.ErrMalformed
	}
	if p[2] == 0 && p[3] == 0 {
		return 0, nil
	}
	if p[2] == 0 || p[2] > game.BagSize {
		return 0, protocol.ErrMalformed
	}
	return p[2], nil
}

// The native stop sender at 0x2c9f8e emits AC23:54 without operands.
// Retain the previously accepted zero-word adapter for compatibility.
func fishingStopRequest(p []byte) error {
	if len(p) == 2 || (len(p) == nativeFishingRequestBytes && p[2] == 0 && p[3] == 0) {
		return nil
	}
	return protocol.ErrMalformed
}

type fishingRun struct {
	Map, X, Y    uint16
	Slot         byte
	Rod          uint16
	Presentation byte
	NextAt       time.Time
	Legacy       bool // AC90 clients receive the source catch receipt after commit.
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
	presentation := nativeFishingDefaultPresentation
	if native, ok := s.Assets.NativeItems[item.ID]; ok {
		if value := native.Record[nativeFishingPresentationOffset]; value > 0 && value <= nativeFishingMaxPresentation {
			presentation = value
		}
	}
	c.fishing = &fishingRun{Map: c.character.Map, X: c.character.X, Y: c.character.Y, Slot: slot, Rod: item.ID, Presentation: presentation, NextAt: due}
	s.broadcastWorld(c, fishingStartedPacket(c))
	return nil
}
func (s *Server) catchFishing(ctx context.Context, c *Session, now time.Time) error {
	run := c.fishing
	if run == nil {
		return nil
	}
	if !s.fishingAvailable(c) || c.character.Map != run.Map || c.character.X != run.X || c.character.Y != run.Y {
		s.stopFishing(c)
		return nil
	}

	if run.Slot == 0 || int(run.Slot) > len(c.character.Bag) {
		s.stopFishing(c)
		return nil
	}
	item := c.character.Bag[run.Slot-1]
	if item.Empty() || item.ID != run.Rod || item.Locked {
		s.stopFishing(c)
		return nil
	}
	if now.Before(run.NextAt) {
		return nil
	}
	f := s.Assets.Fishing
	rod, ok := f.Rod(run.Rod)
	if !f.Enabled || !ok {
		s.stopFishing(c)
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
		adds, err = next.Bag.Grant(game.Item{ID: reward.ItemID}, 1, def.StackLimit(), s.Assets.Items)
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
			packets = append(packets, protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, game.StatSkillEXP, protocol.StatsValueAbsolute}.U32(fishingClientEXP(*sk, f)).U32(uint32(sk.ID)))
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
			s.stopFishing(c)
		}
		// A failed SQL transaction consumes no deadline; retry on maintenance.
		return err
	}
	s.adoptSavedCharacter(c, next)
	run.NextAt = due
	if len(adds) > 0 {
		s.sendOrClose(c, c.character.Bag.AdditionPacket(adds))
		for _, packet := range s.fishingCatchAnimation(c, reward.ItemID) {
			s.sendOrClose(c, packet)
		}
		// Native AC23:51 logs a localized acquisition line in chat. Its record is
		// recipient pet slot (zero for player), item ID and quantity (FUN_003d9984).
		s.sendOrClose(c, protocol.Builder{protocol.CommandInventory, protocol.InventoryAcquisitionNotice, 0}.U16(reward.ItemID).U8(1))
		// Send the catch notification directly to its owner after durable delivery.
		s.sendOrClose(c, headBanner(fmt.Sprintf("Fishing: caught [%s] x1.", s.itemName(reward.ItemID))))
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
		s.stopFishing(c)
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

// Native peer rendering uses AC23:123/122. The owner has already updated its
// local rod slot before sending a request; replaying its stop would make the
// client dereference the cleared slot. Publish these packets only to peers.
func fishingStartedPacket(c *Session) []byte {
	return protocol.Builder{protocol.CommandInventory, protocol.InventoryFishingStarted}.U32(c.character.ID).U8(c.fishing.Presentation)
}
func (s *Server) stopFishing(c *Session) {
	if c.fishing == nil {
		return
	}
	c.fishing = nil
	if c.character != nil {
		s.broadcastWorld(c, protocol.Builder{protocol.CommandInventory, protocol.InventoryFishingStopped}.U32(c.character.ID))
	}
}

// SQL keeps per-grade catches. FUN_00453018 reads cumulative native skill EXP,
// subtracting the previous level's cumulative requirement for its fishing overlay.
func fishingClientEXP(sk game.LearnedSkill, f assets.FishingRules) uint32 {
	exp := uint64(sk.EXP)
	for i := 0; i < int(sk.Grade)-1 && i < len(f.CatchRequirements); i++ {
		exp += uint64(f.CatchRequirements[i])
	}
	return uint32(min(exp, uint64(^uint32(0))))
}

func (s *Server) nativeStatsSnapshot(char game.Character) game.Character {
	snapshot := char.Clone()
	for i := range snapshot.Skills {
		sk := &snapshot.Skills[i]
		for _, id := range s.Assets.Fishing.Skills {
			if sk.ID == id {
				sk.EXP = fishingClientEXP(*sk, s.Assets.Fishing)
				break
			}
		}
	}
	return snapshot
}

// Reuse native ground-item flight (FUN_003dc408 -> FUN_003d73b8, then
// FUN_003dc72c -> FUN_003d74d4 -> FUN_003e0548). AC23:3 allocates the
// client's first free slot; AC23:2 removes it immediately and queues its icon
// flying toward the owner. Avoid a full ground snapshot: its native handler
// replaces objects and resets their count. These two writes run under worldMu.
// This is a presentation adapter, not a verified original fishing reward sequence.
func (s *Server) fishingCatchAnimation(c *Session, item uint16) [][]byte {
	wx, wy, ok := s.Assets.Terrains[c.character.Map].FishingWater(c.character.X, c.character.Y)
	if !ok {
		return nil
	}
	slots, ok := s.World.FreeGroundSlots(c.character.Map, 1)
	if !ok {
		return nil
	}
	var occupied [nativeFishingGroundSlotLimit + 1]bool
	ground := s.World.GroundPacket(c.character.Map)
	for at := 2; at+nativeFishingGroundRecordBytes <= len(ground); at += nativeFishingGroundRecordBytes {
		slot := binary.LittleEndian.Uint16(ground[at+1:])
		if int(slot) < len(occupied) {
			occupied[slot] = true
		}
	}
	slot := 1
	for slot <= nativeFishingGroundSlotLimit && occupied[slot] {
		slot++
	}
	// Taken authored nodes and click aliases may reserve slots absent from the
	// client's snapshot. If its allocator would choose one, skip only the visual.
	if slot != int(slots[0]) {
		return nil
	}
	return [][]byte{
		protocol.Builder{protocol.CommandInventory, protocol.InventoryDrop}.U16(item).U16(wx).U16(wy).U32(0),
		protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(uint16(slot)).U8(protocol.NativeActive),
	}
}
