package store

import "fmt"

// Versioned SQLite DDL retains existing checks, collations and foreign keys.
// Runtime reads and writes use GORM; schema-specific SQL stays in this file.
const schemaVersion = 15

func (s *Store) migrate() error {
	var version int
	if e := s.db.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		return e
	}
	if version == schemaVersion {
		return nil
	}
	if version > schemaVersion {
		return fmt.Errorf("database version %d is newer than supported", version)
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.Exec(`CREATE TABLE IF NOT EXISTS accounts(id INTEGER PRIMARY KEY AUTOINCREMENT CHECK(id < 4490000),username TEXT NOT NULL UNIQUE COLLATE NOCASE,password_hash TEXT NOT NULL,email TEXT NOT NULL DEFAULT '',banned INTEGER NOT NULL DEFAULT 0,im INTEGER NOT NULL DEFAULT 0 CHECK(im>=0),created_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS characters(id INTEGER PRIMARY KEY,account_id INTEGER NOT NULL REFERENCES accounts(id),slot INTEGER NOT NULL CHECK(slot IN (1,2)),name TEXT NOT NULL UNIQUE COLLATE NOCASE,state BLOB NOT NULL,UNIQUE(account_id,slot));
 CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS audit(id INTEGER PRIMARY KEY,at TEXT NOT NULL,action TEXT NOT NULL,subject TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS account_security(account_id INTEGER PRIMARY KEY REFERENCES accounts(id),deletion_hash TEXT NOT NULL);
 PRAGMA user_version=2;`)
	if e != nil {
		return e
	}
	if version < 3 {
		// v3: administrator-granted GM level. Existing accounts start without it.
		var present int
		if e = tx.QueryRow("SELECT count(*) FROM pragma_table_info('accounts') WHERE name='gm_level'").Scan(&present); e != nil {
			return e
		}
		if present == 0 {
			if _, e = tx.Exec("ALTER TABLE accounts ADD COLUMN gm_level INTEGER NOT NULL DEFAULT 0 CHECK(gm_level BETWEEN 0 AND 255)"); e != nil {
				return e
			}
		}
		if _, e = tx.Exec("PRAGMA user_version=3"); e != nil {
			return e
		}
	}
	_, e = tx.Exec(`CREATE TABLE IF NOT EXISTS friendships(
 character1 INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 character2 INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 PRIMARY KEY(character1,character2), CHECK(character1<character2));
 PRAGMA user_version=4;`)
	if e != nil {
		return e
	}
	if version < 5 {
		var present int
		if e = tx.QueryRow("SELECT count(*) FROM pragma_table_info('accounts') WHERE name='im_bonus'").Scan(&present); e != nil {
			return e
		}
		if present == 0 {
			if _, e = tx.Exec("ALTER TABLE accounts ADD COLUMN im_bonus INTEGER NOT NULL DEFAULT 0 CHECK(im_bonus>=0)"); e != nil {
				return e
			}
		}
	}
	// v6: durable native text mail, separate from unsupported parcel mail.
	if _, e = tx.Exec(`CREATE TABLE IF NOT EXISTS text_mail (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 sender_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 sender_name TEXT NOT NULL,
 receiver_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 kind INTEGER NOT NULL CHECK(kind BETWEEN 0 AND 255),
 content BLOB NOT NULL CHECK(length(content) BETWEEN 1 AND 255),
 sent_at_millis INTEGER NOT NULL,
 delivered INTEGER NOT NULL DEFAULT 0 CHECK(delivered IN (0,1)));
 CREATE INDEX IF NOT EXISTS text_mail_pending ON text_mail(receiver_id,delivered,id);
 PRAGMA user_version=6;`); e != nil {
		return e
	}

	// v7: migrated administration records. Gameplay identity foreign keys clean
	// membership, marriages and pending gifts when a character is removed.
	if _, e = tx.Exec(`CREATE TABLE IF NOT EXISTS banned_ips(ip TEXT PRIMARY KEY,reason TEXT NOT NULL,at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS guilds(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,notice TEXT NOT NULL,leader_id INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS guild_members(guild_id INTEGER NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,character_id INTEGER NOT NULL UNIQUE REFERENCES characters(id) ON DELETE CASCADE,PRIMARY KEY(guild_id,character_id));
 CREATE TABLE IF NOT EXISTS marriages(id INTEGER PRIMARY KEY AUTOINCREMENT,character1 INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,character2 INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,at TEXT NOT NULL,CHECK(character1<>character2));
 CREATE TABLE IF NOT EXISTS admin_mail(id INTEGER PRIMARY KEY AUTOINCREMENT,receiver_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,subject TEXT NOT NULL,body TEXT NOT NULL,gold INTEGER NOT NULL CHECK(gold>=0),item_id INTEGER NOT NULL,count INTEGER NOT NULL,claimed INTEGER NOT NULL DEFAULT 0,delivered INTEGER NOT NULL DEFAULT 0,at INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS admin_mail_pending ON admin_mail(receiver_id,delivered,id);
 PRAGMA user_version=7;`); e != nil {
		return e
	}
	// v12: native title and AC66 metadata. Existing typed rows receive zero
	// defaults; create-time v8 projection uses the complete current model.
	if version >= 8 && version < 12 {
		for _, column := range []struct{ Name, DDL string }{
			{"title", "ALTER TABLE character_state ADD COLUMN title INTEGER NOT NULL DEFAULT 0 CHECK(title BETWEEN 0 AND 65535)"},
			{"reborn_job", "ALTER TABLE character_state ADD COLUMN reborn_job INTEGER NOT NULL DEFAULT 0 CHECK(reborn_job BETWEEN 0 AND 255)"},
		} {
			var present int
			if e = tx.QueryRow("SELECT count(*) FROM pragma_table_info('character_state') WHERE name=?", column.Name).Scan(&present); e != nil {
				return e
			}
			if present == 0 {
				if _, e = tx.Exec(column.DDL); e != nil {
					return e
				}
			}
		}
	}

	if version >= 8 && version < 14 {
		for _, column := range []struct{ Table, Name, DDL string }{
			{"character_state", "nickname", "ALTER TABLE character_state ADD COLUMN nickname TEXT NOT NULL DEFAULT ''"},
			{"character_state", "job", "ALTER TABLE character_state ADD COLUMN job INTEGER NOT NULL DEFAULT 0"},
			{"character_state", "potential", "ALTER TABLE character_state ADD COLUMN potential INTEGER NOT NULL DEFAULT 0"},
			{"character_pets", "potential", "ALTER TABLE character_pets ADD COLUMN potential INTEGER NOT NULL DEFAULT 0"},
		} {
			var present int
			if e = tx.QueryRow("SELECT count(*) FROM pragma_table_info(?) WHERE name=?", column.Table, column.Name).Scan(&present); e != nil {
				return e
			}
			if present == 0 {
				if _, e = tx.Exec(column.DDL); e != nil {
					return e
				}
			}
		}
	}
	// v8: typed character state and owned collections; retain the legacy snapshot.
	if e = migrateCharacterState(tx); e != nil {
		return e
	}
	if _, e = tx.Exec("PRAGMA user_version=8"); e != nil {
		return e
	}
	if version < 9 {
		for _, column := range []struct{ Table, Name, DDL string }{
			{"guilds", "icon", "ALTER TABLE guilds ADD COLUMN icon INTEGER NOT NULL DEFAULT 3402"},
			{"guild_members", "rank", "ALTER TABLE guild_members ADD COLUMN rank INTEGER NOT NULL DEFAULT 0 CHECK(rank BETWEEN 0 AND 2)"},
		} {
			var present int
			if e = tx.QueryRow("SELECT count(*) FROM pragma_table_info(?) WHERE name=?", column.Table, column.Name).Scan(&present); e != nil {
				return e
			}
			if present == 0 {
				if _, e = tx.Exec(column.DDL); e != nil {
					return e
				}
			}
		}
		if _, e = tx.Exec(`CREATE TABLE IF NOT EXISTS parcels (
 id INTEGER PRIMARY KEY AUTOINCREMENT CHECK(id BETWEEN 1 AND 4294967295),
 sender_id INTEGER REFERENCES characters(id) ON DELETE SET NULL,
 sender_name TEXT NOT NULL,
 receiver_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
 subject TEXT NOT NULL CHECK(length(subject) BETWEEN 1 AND 100),
 body TEXT NOT NULL,
 gold INTEGER NOT NULL CHECK(gold BETWEEN 0 AND 999999),
 item_id INTEGER NOT NULL CHECK(item_id BETWEEN 0 AND 65535),
 count INTEGER NOT NULL CHECK(count BETWEEN 0 AND 255),
 damage INTEGER NOT NULL CHECK(damage BETWEEN 0 AND 255),
 metadata BLOB NOT NULL CHECK(length(metadata)=26),
 sent_at INTEGER NOT NULL,
 is_read INTEGER NOT NULL DEFAULT 0 CHECK(is_read IN (0,1)),
 claimed INTEGER NOT NULL DEFAULT 0 CHECK(claimed IN (0,1)),
 CHECK((item_id=0 AND count=0) OR (item_id>0 AND count>0)));
 CREATE INDEX IF NOT EXISTS parcels_receiver ON parcels(receiver_id,id);`); e != nil {
			return e
		}
		if _, e = tx.Exec("PRAGMA user_version=9"); e != nil {
			return e
		}
	}

	if version < 10 {
		for _, column := range []string{"tent_return_present", "tent_return_map", "tent_return_x", "tent_return_y"} {
			var present int
			if e = tx.QueryRow("SELECT count(*) FROM pragma_table_info('character_state') WHERE name=?", column).Scan(&present); e != nil {
				return e
			}
			if present == 0 {
				if _, e = tx.Exec("ALTER TABLE character_state ADD COLUMN " + column + " INTEGER NOT NULL DEFAULT 0"); e != nil {
					return e
				}
			}
		}
		if _, e = tx.Exec(`CREATE TABLE IF NOT EXISTS tents(owner_id INTEGER PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,locked INTEGER NOT NULL DEFAULT 0,enlarged INTEGER NOT NULL DEFAULT 0,type INTEGER NOT NULL DEFAULT 0,floor INTEGER NOT NULL,wallpaper INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS tent_items(owner_id INTEGER NOT NULL REFERENCES tents(owner_id) ON DELETE CASCADE,slot INTEGER NOT NULL,item_id INTEGER NOT NULL,count INTEGER NOT NULL,damage INTEGER NOT NULL,metadata BLOB NOT NULL CHECK(length(metadata)=26),x INTEGER NOT NULL,y INTEGER NOT NULL,floor INTEGER NOT NULL,rotation INTEGER NOT NULL,PRIMARY KEY(owner_id,slot));
 PRAGMA user_version=10;`); e != nil {
			return e
		}
	}
	if version < 11 {
		if _, e = tx.Exec(`CREATE TABLE IF NOT EXISTS map_props(map_id INTEGER NOT NULL CHECK(map_id BETWEEN 1 AND 65535),click_id INTEGER NOT NULL CHECK(click_id BETWEEN 1 AND 65535),respawn_at INTEGER NOT NULL,PRIMARY KEY(map_id,click_id));
 PRAGMA user_version=11;`); e != nil {
			return e
		}
	}

	if _, e = tx.Exec(`CREATE TABLE IF NOT EXISTS fishing_progress (
 character_id INTEGER PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,
 catches INTEGER NOT NULL DEFAULT 0 CHECK(catches>=0),
 next_at INTEGER NOT NULL DEFAULT 0);`); e != nil {
		return e
	}

	for _, column := range []string{"floor2", "wallpaper2"} {
		var present int
		if e = tx.QueryRow("SELECT count(*) FROM pragma_table_info('tents') WHERE name=?", column).Scan(&present); e != nil {
			return e
		}
		if present == 0 {
			if _, e = tx.Exec("ALTER TABLE tents ADD COLUMN " + column + " INTEGER NOT NULL DEFAULT 0"); e != nil {
				return e
			}
		}
	}
	if _, e = tx.Exec(`CREATE TABLE IF NOT EXISTS manufacture_jobs(character_id INTEGER PRIMARY KEY REFERENCES characters(id) ON DELETE CASCADE,bench INTEGER NOT NULL,formula INTEGER NOT NULL,item_id INTEGER NOT NULL,count INTEGER NOT NULL,tent_output INTEGER NOT NULL,floor INTEGER NOT NULL,due_at INTEGER NOT NULL,remaining_millis INTEGER NOT NULL,paused INTEGER NOT NULL);`); e != nil {
		return e
	}
	var propFramePresent int
	if e = tx.QueryRow("SELECT count(*) FROM pragma_table_info('map_props') WHERE name='frame'").Scan(&propFramePresent); e != nil {
		return e
	}
	if propFramePresent == 0 {
		if _, e = tx.Exec("ALTER TABLE map_props ADD COLUMN frame INTEGER NOT NULL DEFAULT 1 CHECK(frame BETWEEN 0 AND 1)"); e != nil {
			return e
		}
	}
	if _, e = tx.Exec("PRAGMA user_version=15"); e != nil {
		return e
	}
	return tx.Commit()
}
