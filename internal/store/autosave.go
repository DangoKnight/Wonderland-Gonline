package store

import (
	"context"
	"errors"
	"reflect"

	"gorm.io/gorm"
	"wonderland-go/internal/game"
)

var ErrAutosaveConflict = errors.New("autosave snapshot conflicts with durable character state")

// AutosaveCharacter checkpoints pending session changes without overwriting
// newer durable state. Same-map walking uses a guarded position-only update
// when every other field matches SQL; other changes require the full previous
// checkpoint to match. Already durable or unchanged snapshots need no write.
func (s *Store) AutosaveCharacter(ctx context.Context, ref CharacterRef, baseline, next game.Character) (bool, error) {
	if baseline.ID != ref.ID || next.ID != ref.ID || baseline.Slot != next.Slot || baseline.Name != next.Name {
		return false, errors.New("autosave identity mutation is not allowed")
	}
	if err := next.Validate(); err != nil {
		return false, err
	}
	before := canonicalCharacter(baseline)
	after := canonicalCharacter(next)
	written := false
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		current, err := loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if current.ID != next.ID || current.Slot != next.Slot || current.Name != next.Name {
			return ErrAutosaveConflict
		}
		durable := canonicalCharacter(current)
		if reflect.DeepEqual(durable, after) || reflect.DeepEqual(before, after) {
			return nil
		}
		// Merge only buffered same-map position when every other cached field
		// agrees with authoritative SQL. Never rewrite owned child rows for walking.
		afterState, durableState := after, durable
		afterState.X, afterState.Y = 0, 0
		durableState.X, durableState.Y = 0, 0
		if before.Map == after.Map && durable.Map == before.Map &&
			reflect.DeepEqual(afterState, durableState) && durable.X == before.X && durable.Y == before.Y {
			result := tx.Model(&characterStateRow{}).Where("character_id = ? AND map = ? AND x = ? AND y = ?", ref.ID, before.Map, before.X, before.Y).
				Updates(map[string]any{"x": next.X, "y": next.Y})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAutosaveConflict
			}
			written = true
			return nil
		}
		if !reflect.DeepEqual(durable, before) {
			return ErrAutosaveConflict
		}
		if err := saveCharacter(tx, ref, next); err != nil {
			return err
		}
		written = true
		return nil
	})
	return written && err == nil, err
}

// Canonicalize representation details that JSON comparisons previously ignored.
func canonicalCharacter(c game.Character) game.Character {
	c = c.Clone()
	c.ClearItemLocks()
	c.MutedUntil = c.MutedUntil.Round(0).UTC()
	if len(c.EventTimers) == 0 {
		c.EventTimers = nil
	}
	for id, t := range c.EventTimers {
		c.EventTimers[id] = t.Round(0).UTC()
	}
	if len(c.ChestRespawns) == 0 {
		c.ChestRespawns = nil
	}
	for id, t := range c.ChestRespawns {
		c.ChestRespawns[id] = t.Round(0).UTC()
	}
	for id, q := range c.Quests {
		q.StartedAt = q.StartedAt.Round(0).UTC()
		if q.CompletedAt != nil {
			t := q.CompletedAt.Round(0).UTC()
			q.CompletedAt = &t
		}
		c.Quests[id] = q
	}
	if len(c.Pets) == 0 {
		c.Pets = nil
	}
	if len(c.ReservePets) == 0 {
		c.ReservePets = nil
	}
	if len(c.HotelPets) == 0 {
		c.HotelPets = nil
	}
	return c
}

// CharacterSnapshotsEqual ignores transient reservations and representation
// differences so a clean online session needs no SQL checkpoint transaction.
func CharacterSnapshotsEqual(a, b game.Character) bool {
	return reflect.DeepEqual(canonicalCharacter(a), canonicalCharacter(b))
}
