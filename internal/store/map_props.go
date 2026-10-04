package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
	"wonderland-go/internal/game"
)

const maxMapPropCooldown = 24 * time.Hour

var ErrPropEmpty = errors.New("this node is empty until it respawns")

type MapProp struct {
	MapID     uint16 `gorm:"primaryKey;autoIncrement:false"`
	ClickID   uint16 `gorm:"primaryKey;autoIncrement:false"`
	RespawnAt int64
	Frame     byte
}

func (MapProp) TableName() string { return "map_props" }

// ClaimMapProp claims the shared node and grants its complete reward together.
// A full bag or failed write leaves the node available to every player.
func (s *Store) ClaimMapProp(ctx context.Context, ref CharacterRef, mapID, click uint16, item game.Item, maxStack byte, now time.Time, cooldown time.Duration, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, []game.Addition, error) {
	if mapID == 0 || click == 0 || cooldown <= 0 || cooldown > maxMapPropCooldown || item.ID == 0 || item.Count == 0 || maxStack == 0 {
		return game.Character{}, nil, errors.New("invalid prop claim")
	}
	var next game.Character
	var adds []game.Addition
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var state MapProp
		err := tx.Where("map_id = ? AND click_id = ?", mapID, click).Take(&state).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && now.UnixMilli() < state.RespawnAt {
			return ErrPropEmpty
		}
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if next.Map != mapID {
			return errors.New("character left the prop map")
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		adds, err = next.Bag.Grant(item, int(item.Count), maxStack, items)
		if err != nil {
			return err
		}
		if err = next.Validate(); err != nil {
			return err
		}
		if err = saveCharacter(tx, ref, next); err != nil {
			return err
		}
		state = MapProp{MapID: mapID, ClickID: click, Frame: sharedPropBrokenFrame, RespawnAt: now.Add(cooldown).UnixMilli()}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "map_id"}, {Name: "click_id"}}, DoUpdates: clause.AssignmentColumns([]string{"respawn_at", "frame"})}).Create(&state).Error
	})
	if err != nil {
		return game.Character{}, nil, err
	}
	return next, adds, nil
}
func (s *Store) ActiveMapProps(ctx context.Context, mapID uint16, now time.Time) ([]MapProp, error) {
	var out []MapProp
	err := s.orm.WithContext(ctx).Where("map_id = ? AND respawn_at > ?", mapID, now.UnixMilli()).Order("click_id").Find(&out).Error
	return out, err
}
func (s *Store) ExpireMapProps(ctx context.Context, now time.Time) ([]MapProp, error) {
	var out []MapProp
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("respawn_at <= ?", now.UnixMilli()).Order("map_id,click_id").Find(&out).Error; err != nil {
			return err
		}
		if len(out) == 0 {
			return nil
		}
		return tx.Where("respawn_at <= ?", now.UnixMilli()).Delete(&MapProp{}).Error
	})
	return out, err
}

const sharedPropBrokenFrame = 1

// SetMapPropState persists a scripted renewable prop's frame and reset deadline.
// Quest-specific frames remain per character and never use this shared record.
func (s *Store) SetMapPropState(ctx context.Context, mapID, click uint16, frame byte, now time.Time, cooldown time.Duration) error {
	if mapID == 0 || click == 0 || frame > sharedPropBrokenFrame || cooldown <= 0 || cooldown > maxMapPropCooldown {
		return errors.New("invalid shared prop state")
	}
	row := MapProp{MapID: mapID, ClickID: click, Frame: frame, RespawnAt: now.Add(cooldown).UnixMilli()}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "map_id"}, {Name: "click_id"}}, DoUpdates: clause.AssignmentColumns([]string{"frame", "respawn_at"})}).Create(&row).Error
	})
}
