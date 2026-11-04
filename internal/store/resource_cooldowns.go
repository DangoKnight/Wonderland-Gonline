package store

import (
	"context"
	"database/sql"
	"errors"
	"gorm.io/gorm"
	"time"
	"wonderland-gonline/internal/game"
)

type ResourceKind = game.ResourceKind

const (
	ResourceCollectionActor = game.ResourceCollectionActor
	ResourceGroundItem      = game.ResourceGroundItem
	ResourceCollectionArea  = game.ResourceCollectionArea
)

type ResourceKey struct {
	Kind   ResourceKind `gorm:"primaryKey;autoIncrement:false"`
	MapID  uint16       `gorm:"primaryKey;autoIncrement:false"`
	NodeID uint16       `gorm:"primaryKey;autoIncrement:false"`
}

type ResourceCooldown struct {
	ResourceKey `gorm:"embedded"`
	ExpiresAt   int64
}

func (ResourceCooldown) TableName() string { return "resource_cooldowns" }

var ErrResourceUnavailable = errors.New("resource is waiting to respawn")

func migrateResourceCooldowns(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS resource_cooldowns (
kind INTEGER NOT NULL CHECK(kind IN (1,2,3)),
map_id INTEGER NOT NULL CHECK(map_id BETWEEN 1 AND 65535),
node_id INTEGER NOT NULL CHECK(node_id BETWEEN 1 AND 65535),
expires_at INTEGER NOT NULL,
PRIMARY KEY(kind,map_id,node_id));`)
	return err
}

func (key ResourceKey) valid() bool {
	return key.MapID != 0 && key.NodeID != 0 && (key.Kind == ResourceCollectionActor || key.Kind == ResourceGroundItem || key.Kind == ResourceCollectionArea)
}

// The conditional upsert arbitrates competing players before reading inventory.
// A later grant/save error rolls the shared claim back in the same transaction.
func claimResource(tx *gorm.DB, key ResourceKey, now time.Time, cooldown time.Duration) error {
	if !key.valid() || cooldown <= 0 {
		return ErrResourceUnavailable
	}
	result := tx.Exec(`INSERT INTO resource_cooldowns(kind,map_id,node_id,expires_at) VALUES(?,?,?,?)
ON CONFLICT(kind,map_id,node_id) DO UPDATE SET expires_at=excluded.expires_at
WHERE resource_cooldowns.expires_at <= ?`, key.Kind, key.MapID, key.NodeID, now.Add(cooldown).UnixMilli(), now.UnixMilli())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrResourceUnavailable
	}
	return nil
}

func (s *Store) ResourceExpiry(ctx context.Context, key ResourceKey) (time.Time, error) {
	var row ResourceCooldown
	err := s.orm.WithContext(ctx).Where("kind=? AND map_id=? AND node_id=?", key.Kind, key.MapID, key.NodeID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(row.ExpiresAt).UTC(), nil
}

func (s *Store) ActiveResourceCooldowns(ctx context.Context, kind ResourceKind, mapID uint16, now time.Time) ([]ResourceCooldown, error) {
	var rows []ResourceCooldown
	err := s.orm.WithContext(ctx).Where("kind=? AND map_id=? AND expires_at>?", kind, mapID, now.UnixMilli()).Order("node_id").Find(&rows).Error
	return rows, err
}

// ClaimGroundItem commits the pickup and its own shared respawn clock together.
func (s *Store) ClaimGroundItem(ctx context.Context, ref CharacterRef, key ResourceKey, item game.Item, limit byte, now time.Time, cooldown time.Duration, items map[uint16]game.ItemDefinition, reservations ...game.Character) (game.Character, error) {
	var next game.Character
	if key.Kind != ResourceGroundItem {
		return next, ErrResourceUnavailable
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := claimResource(tx, key, now, cooldown); err != nil {
			return err
		}
		var err error
		next, err = loadCharacter(tx, ref)
		if err != nil {
			return err
		}
		if next.Map != key.MapID {
			return ErrResourceUnavailable
		}
		if err = preserveItemReservations(&next, reservations); err != nil {
			return err
		}
		if _, err = next.Bag.Grant(item, int(item.Count), limit, items); err != nil {
			return err
		}
		if err = next.Validate(); err != nil {
			return err
		}
		return saveCharacter(tx, ref, next)
	})
	if err != nil {
		return game.Character{}, err
	}
	return next, nil
}
