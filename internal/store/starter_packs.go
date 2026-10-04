package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"wonderland-go/internal/game"
)

var ErrStarterPackEmpty = errors.New("starter pack is empty")

// GrantStarterPack grants an administrator-requested whole pack from authoritative
// SQL and commits an audit row in the same transaction. Creation grants are saved
// with the new character; login never automatically redelivers a pack.
func (s *Store) GrantStarterPack(ctx context.Context, ref CharacterRef, grants []game.StarterGrant, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	var next game.Character
	var adds []game.Addition
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if len(grants) == 0 {
			return ErrStarterPackEmpty
		}
		for _, grant := range grants {
			def, known := items[grant.ID]
			if !known || grant.ID == 0 || grant.Count <= 0 {
				return game.ErrInvalidItem
			}
			delta, err := next.Bag.Grant(game.Item{ID: grant.ID}, grant.Count, def.StackLimit(), items)
			if err != nil {
				return err
			}
			adds = append(adds, delta...)
		}
		if err = next.Validate(); err != nil {
			return err
		}
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		return adminAudit(tx, "starter pack grant", ref.ID)
	})
	return next, adds, err
}
