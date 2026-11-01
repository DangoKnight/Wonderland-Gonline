package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"sort"
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
	return s.gatherResources(ctx, ref, timer, map[uint16]int{item: 1}, map[uint16]byte{item: maxStack}, now, eligible, items, reservations...)
}

// GatherResources validates and commits every pool result with the shared node
// cooldown. Definitions and the complete authored branch are checked by caller.
func (s *Store) GatherResources(ctx context.Context, ref CharacterRef, timer uint16, rewards map[uint16]int, now time.Time, eligible func(*game.Character) bool, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	limits := map[uint16]byte{}
	for id := range rewards {
		definition, ok := items[id]
		if !ok {
			return game.Character{}, nil, ErrGatheringUnavailable
		}
		limits[id] = definition.StackLimit()
	}
	return s.gatherResources(ctx, ref, timer, rewards, limits, now, eligible, items, reservations...)
}

func (s *Store) gatherResources(ctx context.Context, ref CharacterRef, timer uint16, rewards map[uint16]int, limits map[uint16]byte, now time.Time, eligible func(*game.Character) bool, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	if timer == 0 || len(rewards) == 0 || eligible == nil {
		return game.Character{}, nil, ErrGatheringUnavailable
	}
	ids := make([]int, 0, len(rewards))
	for id, count := range rewards {
		if id == 0 || count <= 0 || limits[id] == 0 || limits[id] > game.MaxItemStack {
			return game.Character{}, nil, ErrGatheringUnavailable
		}
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
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
		free := 0
		for _, it := range next.Bag {
			if it.Empty() {
				free++
			}
		}
		if free < len(rewards) || next.EventTimerActive(timer, now) || !eligible(&next) {
			return ErrGatheringUnavailable
		}
		for _, n := range ids {
			id := uint16(n)
			part, err := next.Bag.Grant(game.Item{ID: id}, rewards[id], limits[id], items)
			if err != nil {
				return err
			}
			adds = append(adds, part...)
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
