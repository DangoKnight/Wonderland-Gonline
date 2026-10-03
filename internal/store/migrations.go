package store

import "fmt"

// Versioned SQLite DDL retains existing checks, collations and foreign keys.
// Runtime reads and writes use GORM; schema-specific SQL stays in this file.
const schemaVersion = 6

func (s *Store) migrate() error {
	var version int
	if e := s.db.QueryRow("PRAGMA user_version").Scan(&version); e != nil {
		return e
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
	return tx.Commit()
}
