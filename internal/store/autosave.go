package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"wonderland-go/internal/game"
)

var ErrAutosaveConflict = errors.New("autosave snapshot conflicts with durable character state")

// AutosaveCharacter saves pending session changes only if the durable character
// still matches the previous checkpoint. Already committed and unchanged session
// snapshots need no write. A newer durable update is never overwritten.
func (s *Store) AutosaveCharacter(ctx context.Context, ref CharacterRef, baseline, next game.Character) (bool, error) {
	if baseline.ID != ref.ID || next.ID != ref.ID || baseline.Slot != next.Slot || baseline.Name != next.Name {
		return false, errors.New("autosave identity mutation is not allowed")
	}
	if err := next.Validate(); err != nil {
		return false, err
	}
	// Clone normalizes nil/empty collections consistently for all three snapshots.
	before, err := json.Marshal(baseline.Clone())
	if err != nil {
		return false, err
	}
	after, err := json.Marshal(next.Clone())
	if err != nil {
		return false, err
	}
	written := false
	err = s.transaction(ctx, func(tx *gorm.DB) error {
		current, err := loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if current.ID != next.ID || current.Slot != next.Slot || current.Name != next.Name {
			return ErrAutosaveConflict
		}
		durable, err := json.Marshal(current.Clone())
		if err != nil {
			return err
		}
		if bytes.Equal(durable, after) || bytes.Equal(before, after) {
			return nil
		}
		if !bytes.Equal(durable, before) {
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
