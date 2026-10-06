package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
	"wonderland-gonline/internal/game"
)

var ErrLuckyDrawLimit = errors.New("daily Lucky Draw limit reached")

// DrawLucky grants from current durable inventory and consumes the character's
// allowance in the same transaction. Caller selects a validated static reward.
func (s *Store) DrawLucky(ctx context.Context, ref CharacterRef, item uint16, quantity int, maxStack byte, now time.Time, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
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
		if !next.LuckyDraw.Consume(now) {
			return ErrLuckyDrawLimit
		}
		adds, err = next.Bag.Grant(game.Item{ID: item}, quantity, maxStack, items)
		if err != nil {
			return err
		}
		if err := next.Validate(); err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	if errors.Is(err, ErrLuckyDrawLimit) {
		return next, nil, err
	}
	if err != nil {
		return game.Character{}, nil, err
	}
	return next, adds, nil
}
