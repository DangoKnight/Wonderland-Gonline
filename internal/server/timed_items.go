package server

import (
	"context"
	"errors"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

func timedMinutes(seconds uint32) uint32 {
	return uint32((uint64(seconds) + game.TimedMinuteSeconds - 1) / game.TimedMinuteSeconds)
}
func timedPackets(e game.TimedEffects, notice bool) [][]byte {
	enabled := byte(protocol.TimedEffectRunning)
	var out [][]byte
	switch e.EXPKind {
	case game.TimedItemStatue:
		out = append(out, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateStatueTime, enabled}.U32(timedMinutes(e.EXPSeconds)))
	case game.TimedItemPotion:
		if notice {
			out = append(out, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateEXPPotion, protocol.TimedPotionActivated, enabled}.U32(timedMinutes(e.EXPSeconds)).U8(byte(e.EXPTenths)))
		} else {
			out = append(out, []byte{protocol.CommandCharacterState, protocol.CharacterStateEXPPotion, protocol.TimedEffectInitial, enabled, byte(e.EXPTenths)})
		}
	}
	if e.EXPPaused {
		out = append(out, []byte{protocol.CommandInventory, protocol.InventoryStopwatchStatus, protocol.StopwatchStatusInitial, 1})
	}
	if e.TrainingSeconds > 0 {
		notify := byte(0)
		if notice {
			notify = protocol.TimedNotify
		}
		out = append(out, protocol.Builder{protocol.CommandInventory, protocol.InventoryTrainingTime, protocol.TrainingTimeUpdate, protocol.TrainingPassStandard}.U16(uint16(min(timedMinutes(e.TrainingSeconds), 1<<16-1))).U8(notify))
	}
	return out
}

// Called under worldMu. Online seconds are recoverable session state and are
// checkpointed with walking; consumption/activation always commits immediately.
func (s *Server) advanceTimedSession(c *Session, now time.Time, publish bool) {
	if c == nil || c.character == nil {
		return
	}
	if c.timedTickAt.IsZero() {
		c.timedTickAt = now
		return
	}
	seconds := uint32(max(0, int64(now.Sub(c.timedTickAt)/time.Second)))
	if seconds == 0 {
		return
	}
	c.timedTickAt = c.timedTickAt.Add(time.Duration(seconds) * time.Second)
	before := c.character.TimedEffects
	c.character.TimedEffects.Advance(seconds, s.Assets.Maps[c.character.Map].TrainingArea)
	after := c.character.TimedEffects
	if !publish {
		return
	}
	if before.EXPSeconds > 0 && after.EXPSeconds == 0 {
		if before.EXPKind == game.TimedItemPotion {
			s.sendOrClose(c, []byte{protocol.CommandCharacterState, protocol.CharacterStateEXPPotion, protocol.TimedPotionExpired, 0})
		} else {
			s.sendOrClose(c, []byte{protocol.CommandCharacterState, protocol.CharacterStateStatueTime, protocol.TimedStatueExpired, 1})
		}
	}
	if before.TrainingSeconds > 0 && after.TrainingSeconds == 0 {
		s.sendOrClose(c, []byte{protocol.CommandInventory, protocol.InventoryTrainingTime, protocol.TrainingTimeExpired})
	} else if timedMinutes(before.TrainingSeconds) != timedMinutes(after.TrainingSeconds) {
		// Training status refresh is quiet, exactly as in the Legacy capture.
		s.sendOrClose(c, protocol.Builder{protocol.CommandInventory, protocol.InventoryTrainingTime, protocol.TrainingTimeUpdate, protocol.TrainingPassStandard}.U16(uint16(min(timedMinutes(after.TrainingSeconds), 1<<16-1))).U8(0))
	}
}
func (s *Server) trainingEligible(c *Session) bool {
	s.advanceTimedSession(c, time.Now(), true)
	return !s.Assets.Maps[c.character.Map].TrainingArea || c.character.TimedEffects.TrainingSeconds > 0
}
func (s *Server) trainingRewardEligible(c *Session, run *battleRun) bool {
	s.advanceTimedSession(c, time.Now(), true)
	return !s.battleMapDefinition(run).TrainingArea || c.character.TimedEffects.TrainingSeconds > 0
}

func (s *Server) useTimedItem(ctx context.Context, c *Session, slot byte) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	d := s.Assets.Items[item.ID]
	if d.TimedKind == game.TimedItemNone {
		return false, nil
	}
	s.advanceTimedSession(c, time.Now(), true)
	if d.TimedKind == game.TimedItemStopwatch && c.character.TimedEffects.EXPPaused {
		return true, s.resumeEXPClock(ctx, c)
	}
	trial := c.character.TimedEffects
	if err := trial.Activate(d); err != nil {
		reason := byte(protocol.TimedRejectRepeated)
		if errors.Is(err, game.ErrTimedIncompatible) {
			reason = protocol.TimedRejectIncompatible
		}
		if errors.Is(err, game.ErrTimedUnavailable) {
			return true, c.send(serverwire.HeadBanner(err.Error()))
		}
		return true, c.send([]byte{protocol.CommandCharacterState, protocol.CharacterStateEXPPotion, protocol.TimedEffectRejected, reason})
	}
	if item.Locked {
		return true, c.send(serverwire.HeadBanner(game.ErrItemLocked.Error()))
	}
	if err := s.autosaveSession(ctx, c); err != nil {
		return true, err
	}
	expected := c.character.TimedEffects
	var saved game.Character
	err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error {
		if stored.Bag[slot-1] != item || stored.TimedEffects != expected {
			return game.ErrInvalidItem
		}
		if err := stored.TimedEffects.Activate(d); err != nil {
			return err
		}
		if err := stored.Bag.Remove(slot, 1); err != nil {
			return err
		}
		saved = stored.Clone()
		return nil
	})
	if err != nil {
		return true, err
	}
	s.adoptSavedCharacter(c, saved)
	packets := [][]byte{{protocol.CommandInventory, protocol.InventoryRemove, slot, 1}, {protocol.CommandInventory, protocol.InventoryItemUse}}
	if d.TimedKind == game.TimedItemStopwatch {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryStopwatchStatus, protocol.StopwatchStatusChanged, 1})
	} else {
		status := c.character.TimedEffects
		if d.TimedKind == game.TimedItemTraining {
			status.EXPKind = game.TimedItemNone
			status.EXPPaused = false
		} else {
			status.TrainingSeconds = 0
		}
		packets = append(packets, timedPackets(status, true)...)
	}
	return true, s.sendAll(c, packets)
}

func (s *Server) stopwatchControl(ctx context.Context, c *Session, p []byte) error {
	if len(p) != 3 {
		return protocol.ErrMalformed
	}
	if p[2] == 0 {
		return s.resumeEXPClock(ctx, c)
	}
	if p[2] > game.BagSize || s.Assets.Items[c.character.Bag[p[2]-1].ID].TimedKind != game.TimedItemStopwatch {
		return game.ErrInvalidItem
	}
	_, err := s.useTimedItem(ctx, c, p[2])
	return err
}

// Native LAB_0018f8e0 resumes with slot zero. No second Stopwatch is consumed.
func (s *Server) resumeEXPClock(ctx context.Context, c *Session) error {
	s.advanceTimedSession(c, time.Now(), true)
	if !c.character.TimedEffects.EXPPaused {
		return nil
	}
	if err := s.autosaveSession(ctx, c); err != nil {
		return err
	}
	expected := c.character.TimedEffects
	var saved game.Character
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error {
		if stored.TimedEffects != expected {
			return game.ErrInvalidItem
		}
		stored.TimedEffects.EXPPaused = false
		saved = stored.Clone()
		return nil
	}); err != nil {
		return err
	}
	s.adoptSavedCharacter(c, saved)
	return c.send([]byte{protocol.CommandInventory, protocol.InventoryStopwatchStatus, protocol.StopwatchStatusChanged, 0})
}
func (s *Server) trainingControl(ctx context.Context, c *Session, p []byte) error {
	if len(p) != 3 || p[2] != protocol.TrainingControlEnter {
		return protocol.ErrMalformed
	}
	s.advanceTimedSession(c, time.Now(), true)
	if c.character.TimedEffects.TrainingSeconds == 0 {
		return c.send(serverwire.HeadBanner("A valid Training Pass is required."))
	}
	if !commandTravelAvailable(c) || c.party != nil && c.party.members[0] != c {
		return c.send(serverwire.HeadBanner("Cannot travel to Training Island right now."))
	}
	var destination world.Destination
	for id, m := range s.Assets.Maps {
		if m.TrainingArea && m.TrainingEntryX != 0 && m.TrainingEntryY != 0 && (destination.Map == 0 || id < destination.Map) {
			destination = world.Destination{Map: id, X: m.TrainingEntryX, Y: m.TrainingEntryY}
		}
	}
	if destination.Map != 0 {
		return s.teleport(ctx, c, destination, 0)
	}
	return c.send(serverwire.HeadBanner("Training Island is unavailable."))
}
