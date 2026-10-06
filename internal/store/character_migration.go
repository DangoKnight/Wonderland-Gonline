package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"wonderland-gonline/internal/game"
)

// Migration is additive. The old JSON column is retained as a migration snapshot;
// runtime never reads it or updates it after a character has structured state.
func migrateCharacterState(conn *sql.Tx) error {
	tx, err := gorm.Open(sqlite.New(sqlite.Config{Conn: conn}), &gorm.Config{SkipDefaultTransaction: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	for _, model := range characterTables() {
		if !tx.Migrator().HasTable(model) {
			if err = tx.Migrator().CreateTable(model); err != nil {
				return err
			}
		}
	}
	var rows []characterRow
	if err = tx.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var count int64
		if err = tx.Model(&characterStateRow{}).Where("character_id = ?", row.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			continue
		} // Preserve migrated values when upgrading other tables.
		var c game.Character
		if err = json.Unmarshal(row.State, &c); err != nil {
			return fmt.Errorf("migrate character %d: %w", row.ID, err)
		}
		if c.ID != row.ID || c.Slot != row.Slot || c.Name != row.Name {
			return fmt.Errorf("migrate character %d: identity differs from stored state", row.ID)
		}
		if err = writeCharacterState(tx, c); err != nil {
			return fmt.Errorf("migrate character %d: %w", row.ID, err)
		}
		projected, err := readCharacterState(tx, row)
		if err != nil {
			return err
		}
		if CharacterVersion(c) != CharacterVersion(projected) {
			return fmt.Errorf("migrate character %d: conversion changed state", row.ID)
		}
	}
	return nil
}
