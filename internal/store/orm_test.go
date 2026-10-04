package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"wonderland-go/internal/game"
)

func TestORMOpensExistingDatabaseWithoutChangingSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy-go.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	// Build the current Go schema independently of GORM's models/migrations.
	_, err = legacy.Exec(`CREATE TABLE accounts(id INTEGER PRIMARY KEY AUTOINCREMENT CHECK(id<4490000),username TEXT NOT NULL UNIQUE COLLATE NOCASE,password_hash TEXT NOT NULL,email TEXT NOT NULL DEFAULT '',banned INTEGER NOT NULL DEFAULT 0,im INTEGER NOT NULL DEFAULT 0 CHECK(im>=0),created_at TEXT NOT NULL,gm_level INTEGER NOT NULL DEFAULT 0 CHECK(gm_level BETWEEN 0 AND 255),im_bonus INTEGER NOT NULL DEFAULT 0 CHECK(im_bonus>=0));
 CREATE TABLE characters(id INTEGER PRIMARY KEY,account_id INTEGER NOT NULL REFERENCES accounts(id),slot INTEGER NOT NULL CHECK(slot IN (1,2)),name TEXT NOT NULL UNIQUE COLLATE NOCASE,state BLOB NOT NULL,UNIQUE(account_id,slot));
 CREATE TABLE account_security(account_id INTEGER PRIMARY KEY REFERENCES accounts(id),deletion_hash TEXT NOT NULL);
 CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE audit(id INTEGER PRIMARY KEY,at TEXT NOT NULL,action TEXT NOT NULL,subject TEXT NOT NULL);
 CREATE TABLE friendships(character1 INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,character2 INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,PRIMARY KEY(character1,character2),CHECK(character1<character2));
 CREATE TABLE text_mail(id INTEGER PRIMARY KEY AUTOINCREMENT,sender_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,sender_name TEXT NOT NULL,receiver_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,kind INTEGER NOT NULL CHECK(kind BETWEEN 0 AND 255),content BLOB NOT NULL CHECK(length(content) BETWEEN 1 AND 255),sent_at_millis INTEGER NOT NULL,delivered INTEGER NOT NULL DEFAULT 0 CHECK(delivered IN (0,1)));
 CREATE INDEX text_mail_pending ON text_mail(receiver_id,delivered,id);
 PRAGMA user_version=6;`)
	if err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	hash, err := hashPassword("old-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("INSERT INTO accounts(id,username,password_hash,created_at,gm_level) VALUES(7,'Legacy',?,'2026-01-01',2)", hash); err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: 10007, Slot: 1, Name: "LegacyPlayer", Level: 1, HP: 10, MaxHP: 10, Gold: 50}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("INSERT INTO characters(id,account_id,slot,name,state) VALUES(10007,7,1,'LegacyPlayer',?)", raw); err != nil {
		t.Fatal(err)
	}
	schema := func(db *sql.DB) map[string]string {
		t.Helper()
		rows, err := db.Query("SELECT name,sql FROM sqlite_master WHERE type='table'")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := map[string]string{}
		for rows.Next() {
			var name, ddl string
			if err := rows.Scan(&name, &ddl); err != nil {
				t.Fatal(err)
			}
			result[name] = ddl
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := schema(legacy)
	legacy.Close()
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	after := schema(db.db)
	for name, ddl := range before {
		if after[name] != ddl {
			t.Fatalf("ORM changed existing table %s", name)
		}
	}
	if after["admin_mail"] == "" || after["banned_ips"] == "" {
		t.Fatal("administration tables missing after upgrade")
	}
	a, err := db.Authenticate(ctx, "legacy", "old-secret")
	if err != nil || a.ID != 7 || a.GMLevel != 2 {
		t.Fatal("legacy credentials or grants changed", a, err)
	}
	chars, err := db.Characters(ctx, a.ID)
	if err != nil || len(chars) != 1 || chars[0].Gold != 50 {
		t.Fatal("legacy state unreadable", chars, err)
	}
	if available, err := db.CharacterNameAvailable(ctx, "legacyplayer"); err != nil || available {
		t.Fatal("case-insensitive name reservation lost", err)
	}
	if _, err := db.Register(ctx, "LEGACY", "password", ""); err != ErrConflict {
		t.Fatal("case-insensitive account uniqueness lost", err)
	}
	next, err := db.Register(ctx, "nextuser", "password", "")
	if err != nil || next.ID != 8 {
		t.Fatal("account sequence changed", next, err)
	}
	if err := db.UpdateCharacter(ctx, a.ID, c.ID, func(c *game.Character) error { c.Gold = 0; return nil }); err != nil {
		t.Fatal(err)
	}
	chars, err = db.Characters(ctx, a.ID)
	if err != nil || len(chars) != 1 || chars[0].Gold != 0 {
		t.Fatal("JSON zero update not saved", err)
	}
}

func TestORMZeroValueUpdatesAndUpserts(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, err := db.Register(ctx, "tester", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []bool{true, false} {
		if err := db.SetBanned(ctx, a.ID, banned); err != nil {
			t.Fatal(err)
		}
	}
	for _, level := range []byte{2, 0} {
		if err := db.SetGMLevel(ctx, a.ID, level); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.Authenticate(ctx, "tester", "password")
	if err != nil || got.Banned || got.GMLevel != 0 {
		t.Fatal("ORM skipped zero/false update", got, err)
	}
	for _, motd := range []string{"Welcome", ""} {
		if err := db.SaveSettings(ctx, map[string]string{"motd": motd}); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := db.Settings(ctx)
	motd, exists := settings["motd"]
	if err != nil || !exists || motd != "" {
		t.Fatal("ORM upsert skipped empty string", settings, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := db.UpdateCharacter(cancelled, a.ID, a.CharacterID(1), func(*game.Character) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled transaction did not return context cancellation", err)
	}
}
