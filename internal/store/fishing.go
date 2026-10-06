package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"wonderland-gonline/internal/game"
)

var ErrFishingNotDue = errors.New("fishing catch is not due")

type fishingProgress struct {
	CharacterID uint32 `gorm:"primaryKey;autoIncrement:false"`
	Catches     uint32
	NextAt      int64
}

func (fishingProgress) TableName() string { return "fishing_progress" }

// MutateFishing serializes rod ownership, the durable deadline, progress and
// reward delivery. A caller cannot replay a deadline or reset it by recasting.
func (s *Store) MutateFishing(ctx context.Context, ref CharacterRef, slot byte, rod uint16, now time.Time, interval time.Duration, catch bool, fn func(*game.Character) error, reservations ...game.Character) (game.Character, time.Time, error) {
	var next game.Character
	var due time.Time
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if (catch && fn == nil) || interval <= 0 || slot == 0 || int(slot) > len(next.Bag) {
			return game.ErrInvalidItem
		}
		item := next.Bag[slot-1]
		if item.Empty() || item.ID != rod || item.Locked {
			return game.ErrInvalidItem
		}
		progress := fishingProgress{CharacterID: ref.ID}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&progress).Error; err != nil {
			return err
		}
		if err = tx.First(&progress, ref.ID).Error; err != nil {
			return err
		}
		due = time.UnixMilli(progress.NextAt)
		if catch {
			if progress.NextAt == 0 || now.UnixMilli() < progress.NextAt {
				return ErrFishingNotDue
			}
			if err = fn(&next); err != nil {
				return err
			}
			if err = next.Validate(); err != nil {
				return err
			}
			if err = saveCharacter(tx, ref, next); err != nil {
				return err
			}
			if progress.Catches < ^uint32(0) {
				progress.Catches++
			}
			progress.NextAt = now.Add(interval).UnixMilli()
		} else if progress.NextAt <= now.UnixMilli() {
			progress.NextAt = now.Add(interval).UnixMilli()
		}
		due = time.UnixMilli(progress.NextAt)
		return tx.Save(&progress).Error
	})
	return next, due, err
}
