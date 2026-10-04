package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"wonderland-go/internal/game"
)

// MutateOwnedCharacter validates resources from SQL, then publishes a committed
// snapshot. Caller owns the session lock; pending movement is merged on adoption.
func (s *Store) MutateOwnedCharacter(ctx context.Context, ref CharacterRef, fn func(*game.Character) error, reservations ...game.Character) (game.Character, error) {
	var next game.Character
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		id, slot, name := next.ID, next.Slot, next.Name
		if err = fn(&next); err != nil {
			return err
		}
		if next.ID != id || next.Slot != slot || next.Name != name {
			return errors.New("identity mutation is not allowed")
		}
		if err = next.Validate(); err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	return next, err
}
