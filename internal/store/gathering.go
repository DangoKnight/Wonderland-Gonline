package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
	"wonderland-gonline/internal/game"
)

var ErrGatheringUnavailable = errors.New("resource gathering conditions are not met")

// GatherWater rechecks the branch against current durable state and commits the
// item, timer and quest mark atomically. eligible must be read-only.
func (s *Store) GatherWater(ctx context.Context, ref CharacterRef, timer, item uint16, maxStack byte, now time.Time, eligible func(*game.Character) bool, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	expected, supported := game.WaterGatheringItem(timer)
	if !supported || item != expected || maxStack == 0 || maxStack > game.MaxItemStack || eligible == nil {
		return game.Character{}, nil, ErrGatheringUnavailable
	}
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
		free := false
		for _, it := range next.Bag {
			if it.Empty() {
				free = true
				break
			}
		}
		if !free || next.EventTimerActive(timer, now) || !eligible(&next) {
			return ErrGatheringUnavailable
		}
		adds, err = next.Bag.Grant(game.Item{ID: item}, 1, maxStack, items)
		if err != nil {
			return err
		}
		if next.EventTimers == nil {
			next.EventTimers = make(map[uint16]time.Time)
		}
		next.EventTimers[timer] = now.Add(game.WaterGatheringCooldown).UTC()
		if next.Quests == nil {
			next.Quests = make(map[uint32]game.Quest)
		}
		next.Quests[uint32(timer)] = game.Quest{ID: uint32(timer), State: game.InProgress, Step: 1, StartedAt: now.UTC()}
		if err := next.Validate(); err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	if err != nil {
		return game.Character{}, nil, err
	}
	return next, adds, nil
}
