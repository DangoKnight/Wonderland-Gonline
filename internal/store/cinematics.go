package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const cinematicIDMaxBytes = 128

type cinematicCompletion struct {
	CharacterID uint32 `gorm:"primaryKey;autoIncrement:false"`
	CinematicID string `gorm:"primaryKey"`
	CompletedAt int64
}

func (cinematicCompletion) TableName() string { return "character_cinematics" }

func validCinematicID(id string) bool { return len(id) > 0 && len(id) <= cinematicIDMaxBytes }

func (s *Store) CinematicCompleted(ctx context.Context, ref CharacterRef, id string) (bool, error) {
	if !validCinematicID(id) {
		return false, errors.New("invalid cinematic ID")
	}
	var owner characterRow
	if err := s.orm.WithContext(ctx).Select("id").Where("id = ? AND account_id = ?", ref.ID, ref.Account).Take(&owner).Error; err != nil {
		return false, persistenceError(err)
	}
	var count int64
	err := s.orm.WithContext(ctx).Model(&cinematicCompletion{}).Where("character_id = ? AND cinematic_id = ?", ref.ID, id).Count(&count).Error
	return count > 0, err
}

// CompleteCinematic writes an idempotent presentation receipt without touching
// quest progression, inventory or buffered character state.
func (s *Store) CompleteCinematic(ctx context.Context, ref CharacterRef, id string, now time.Time) error {
	if !validCinematicID(id) {
		return errors.New("invalid cinematic ID")
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var owner characterRow
		if err := tx.Select("id").Where("id = ? AND account_id = ?", ref.ID, ref.Account).Take(&owner).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cinematicCompletion{ref.ID, id, now.UnixMilli()}).Error
	})
}

func migrateCinematics(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS character_cinematics (
 character_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 cinematic_id TEXT NOT NULL CHECK(length(cinematic_id) BETWEEN 1 AND 128),
 completed_at INTEGER NOT NULL,
 PRIMARY KEY(character_id, cinematic_id));`)
	return err
}
