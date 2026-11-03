package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"wonderland-gonline/internal/game"
)

// UpdateCharacters commits all recipients of one combat acquisition atomically.
func (s *Store) UpdateCharacters(ctx context.Context, refs []CharacterRef, fn func([]game.Character) error) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		chars := make([]game.Character, len(refs))
		before := make([]game.Character, len(refs))
		seen := map[uint32]bool{}
		for i, ref := range refs {
			if ref.ID == 0 || seen[ref.ID] {
				return errors.New("distinct characters required")
			}
			seen[ref.ID] = true
			c, err := loadCharacter(tx, ref)
			if err != nil {
				return err
			}
			chars[i] = c
			before[i] = c.Clone()
		}
		if err := fn(chars); err != nil {
			return err
		}
		for i, c := range chars {
			if c.ID != before[i].ID || c.Slot != before[i].Slot || c.Name != before[i].Name {
				return errors.New("identity mutation is not allowed")
			}
			if err := c.Validate(); err != nil {
				return err
			}
			if err := saveCharacter(tx, refs[i], c); err != nil {
				return err
			}
		}
		return nil
	})
}
