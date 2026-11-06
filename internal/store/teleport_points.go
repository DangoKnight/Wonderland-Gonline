package store

import (
	"context"
	"database/sql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"wonderland-gonline/internal/game"
)

const MaxTeleportPoints = 3

type teleportPointRow struct {
	CharacterID uint32 `gorm:"primaryKey;autoIncrement:false"`
	Point       byte   `gorm:"primaryKey;autoIncrement:false"`
	Map, X, Y   uint16
}

func (teleportPointRow) TableName() string { return "character_teleport_points" }

func (s *Store) TeleportPoints(ctx context.Context, ref CharacterRef) (map[byte]game.Location, error) {
	result := map[byte]game.Location{}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if _, err := loadCharacter(tx, ref); err != nil {
			return err
		}
		var rows []teleportPointRow
		if err := tx.Where("character_id = ?", ref.ID).Order("point").Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			result[r.Point] = game.Location{Map: r.Map, X: r.X, Y: r.Y}
		}
		return nil
	})
	return result, err
}

// RecordTeleportPoint verifies the scroll and current map against SQL before
// recording the session's current (possibly buffered) walking position.
func (s *Store) RecordTeleportPoint(ctx context.Context, ref CharacterRef, point, slot byte, location game.Location, scrolls map[uint16]byte, reservations ...game.Character) error {
	if point < 1 || point > MaxTeleportPoints || slot < 1 || slot > game.BagSize || location.Map == 0 || location.Map == game.TentMapID {
		return game.ErrInvalidItem
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		c, err := loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if err = preserveItemReservations(&c, reservations); err != nil {
			return err
		}
		item := c.Bag[slot-1]
		if c.Map != location.Map || item.Empty() || item.Locked || scrolls[item.ID] < point {
			return game.ErrInvalidItem
		}
		row := teleportPointRow{CharacterID: ref.ID, Point: point, Map: location.Map, X: location.X, Y: location.Y}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "character_id"}, {Name: "point"}}, DoUpdates: clause.AssignmentColumns([]string{"map", "x", "y"})}).Create(&row).Error
	})
}

// migrateTeleportPoints preserves existing owned state while upgrading its SQL schema.
func migrateTeleportPoints(tx *sql.Tx) error {
	if _, e := tx.Exec(`CREATE TABLE IF NOT EXISTS character_teleport_points (
 character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 point INTEGER NOT NULL CHECK(point BETWEEN 1 AND 3),
 map INTEGER NOT NULL CHECK(map BETWEEN 1 AND 65535),
 x INTEGER NOT NULL CHECK(x BETWEEN 0 AND 65535),
 y INTEGER NOT NULL CHECK(y BETWEEN 0 AND 65535),
 PRIMARY KEY(character_id, point));`); e != nil {
		return e
	}
	return nil
}
