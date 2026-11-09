package store

import (
	"database/sql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func migrateTimedEffects(conn *sql.Tx) error {
	tx, err := gorm.Open(sqlite.New(sqlite.Config{Conn: conn}), &gorm.Config{SkipDefaultTransaction: true, DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	if !tx.Migrator().HasTable(&characterStateRow{}) {
		return nil
	}
	for _, field := range []string{"EXPKind", "EXPTenths", "EXPSeconds", "EXPPaused", "TrainingSeconds"} {
		// Embedded fields retain their Go names in GORM's schema.
		if !tx.Migrator().HasColumn(&characterStateRow{}, field) {
			if err = tx.Migrator().AddColumn(&characterStateRow{}, field); err != nil {
				return err
			}
		}
	}
	return nil
}
