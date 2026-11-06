package store

import "database/sql"

func migratePetFood(tx *sql.Tx) error {
	var count int
	if err := tx.QueryRow("SELECT count(*) FROM pragma_table_info('character_pets') WHERE name='stat_food_uses'").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	_, err := tx.Exec("ALTER TABLE character_pets ADD COLUMN stat_food_uses INTEGER NOT NULL DEFAULT 0 CHECK(stat_food_uses BETWEEN 0 AND 1200)")
	return err
}
