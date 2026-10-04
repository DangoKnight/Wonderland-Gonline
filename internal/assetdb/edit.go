package assetdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
)

var ErrEditConflict = errors.New("asset changed; refresh before saving")

func DocumentVersion(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

// ReplaceDocument atomically updates authoritative indexed rows and validates the
// full candidate catalog before committing. Source provenance remains unchanged.
func ReplaceDocument(ctx context.Context, db *gorm.DB, asset string, version string, raw []byte, validate func(*gorm.DB) error) error {
	if !json.Valid(raw) {
		return errors.New("invalid asset JSON")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, err := ReadDocument(tx, asset)
		if err != nil {
			return err
		}
		if version == "" || DocumentVersion(current) != version {
			return ErrEditConflict
		}
		if err = tx.Model(&Document{}).Where(map[string]any{"asset": asset}).Update("json", string(raw)).Error; err != nil {
			return err
		}
		if err = tx.Where(map[string]any{"asset": asset}).Delete(&Record{}).Error; err != nil {
			return err
		}
		if _, err = importRecords(tx, asset, raw); err != nil {
			return err
		}
		return validate(tx)
	})
}

// EnsureDocument creates only administrator-authored optional tables.
func EnsureDocument(ctx context.Context, db *gorm.DB, asset string) error {
	allowed := asset == "map_overrides.json" || asset == "chest_drops.json" || asset == "quest_visibility.json" || asset == "combat_trials.json"
	if !allowed {
		return errors.New("asset is not an optional administration table")
	}
	var count int64
	if err := db.WithContext(ctx).Model(&Document{}).Where(map[string]any{"asset": asset}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.WithContext(ctx).Create(&Document{Asset: asset, Origin: "administrator", Source: "SQL administration", JSON: `{"schema_version":1,"value":[]}`}).Error
}
func ReplaceRecord(ctx context.Context, db *gorm.DB, asset, collection string, ordinal int, version string, raw []byte, validate func(*gorm.DB) error) error {
	if !json.Valid(raw) {
		return errors.New("invalid record JSON")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row Record
		if err := tx.Where(map[string]any{"asset": asset, "collection": collection, "ordinal": ordinal}).Take(&row).Error; err != nil {
			return err
		}
		if version == "" || DocumentVersion([]byte(row.JSON)) != version {
			return ErrEditConflict
		}
		id, name := recordIdentity(raw)
		if err := tx.Model(&Record{}).Where(map[string]any{"asset": asset, "collection": collection, "ordinal": ordinal}).Updates(map[string]any{"json": string(raw), "game_id": id, "name": name}).Error; err != nil {
			return err
		}
		return validate(tx)
	})
}
