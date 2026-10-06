package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestLegacyImportRollbackPasswordsAndOwnership(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	records := []LegacyAccount{{Account: Account{ID: 7, Username: "Legacy"}, Password: "password", DeletionCode: "123456", Characters: []game.Character{{ID: 10007, Slot: 1, Name: "Hero", Level: 1, Map: 12000}}}}
	if _, err := db.db.Exec("CREATE TRIGGER reject_import BEFORE INSERT ON character_state BEGIN SELECT RAISE(ABORT,'reject'); END"); err != nil {
		t.Fatal(err)
	}
	if err := db.ImportLegacy(ctx, records); err == nil {
		t.Fatal("failed save accepted")
	}
	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial account committed", count, err)
	}
	if _, err := db.db.Exec("DROP TRIGGER reject_import"); err != nil {
		t.Fatal(err)
	}
	if err := db.ImportLegacy(ctx, records, LegacyFriendship{10007, 10008}); err == nil {
		t.Fatal("orphan friendship")
	}
	if err := db.ImportLegacy(ctx, records); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := db.db.QueryRow("SELECT password_hash FROM accounts WHERE id=7").Scan(&hash); err != nil || !strings.HasPrefix(hash, "pbkdf2") || hash == "password" {
		t.Fatal("legacy password persisted", err)
	}
	if err := db.db.QueryRow("SELECT deletion_hash FROM account_security WHERE account_id=7").Scan(&hash); err != nil || !verifyPassword("123456", hash) || hash == "123456" {
		t.Fatal("deletion code not hashed", err)
	}
	if err := db.ImportLegacy(ctx, records); err == nil {
		t.Fatal("existing account overwritten")
	}
}
