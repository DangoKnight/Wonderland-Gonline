package store

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"wonderland-gonline/internal/game"
)

type CharacterRef struct{ Account, ID uint32 }

// UpdateCharacterPair commits both characters together, preserving rollback and ownership.
func (s *Store) UpdateCharacterPair(ctx context.Context, a, b CharacterRef, fn func(*game.Character, *game.Character) error) error {
	if a.ID == b.ID || a.ID == 0 || b.ID == 0 {
		return errors.New("distinct characters are required")
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		refs := [2]CharacterRef{a, b}
		var chars [2]game.Character
		for i, ref := range refs {
			var err error
			chars[i], err = loadCharacter(tx, ref)
			if err != nil {
				return err
			}
		}
		before := [2]game.Character{chars[0].Clone(), chars[1].Clone()}
		if err := fn(&chars[0], &chars[1]); err != nil {
			return err
		}
		for i, ref := range refs {
			c := chars[i]
			if c.ID != before[i].ID || c.Slot != before[i].Slot || c.Name != before[i].Name {
				return errors.New("identity mutation is not allowed")
			}
			if err := c.Validate(); err != nil {
				return err
			}
			if err := saveCharacter(tx, ref, c); err != nil {
				return err
			}
		}
		return nil
	})
}
