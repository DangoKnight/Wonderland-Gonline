package dbmigration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

func gameplayFixture(t *testing.T, path string, gold uint32) {
	t.Helper()
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.Register(context.Background(), "tester", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: a.CharacterID(1), Name: "Tester", Slot: 1, Level: 1, Gold: gold}
	if err = s.CreateCharacter(context.Background(), a, c); err != nil {
		t.Fatal(err)
	}
}

func goldAt(t *testing.T, path string) uint32 {
	t.Helper()
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.Authenticate(context.Background(), "tester", "password")
	if err != nil {
		t.Fatal(err)
	}
	chars, err := s.Characters(context.Background(), a.ID)
	if err != nil || len(chars) != 1 {
		t.Fatal(chars, err)
	}
	return chars[0].Gold
}

// TestUpgradeInPlace: the upgraded databases land at the configured paths
// and the originals move to backups/pre-migrate-<stamp>; no staging files
// remain, and a second run with the same stamp refuses to overwrite.
func TestUpgradeInPlace(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.db"), filepath.Join(root, "b.db")
	gameplayFixture(t, a, 45)
	gameplayFixture(t, b, 7)
	backups, err := UpgradeInPlace([]Target{{Path: a}, {Path: b}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backups", "pre-migrate-test")
	if len(backups) != 1 || backups[0] != backup {
		t.Fatalf("backups %v", backups)
	}
	if goldAt(t, a) != 45 || goldAt(t, b) != 7 {
		t.Fatal("upgraded data differs")
	}
	if goldAt(t, filepath.Join(backup, "a.db")) != 45 {
		t.Fatal("original not kept")
	}
	if _, err = os.Stat(filepath.Join(root, ".migrate-test")); !os.IsNotExist(err) {
		t.Fatal("staging left behind", err)
	}
	if _, err = UpgradeInPlace([]Target{{Path: a}}, "test"); err == nil {
		t.Fatal("overwrote an existing backup")
	}
	if goldAt(t, a) != 45 {
		t.Fatal("failed run damaged the database")
	}
}

// TestUpgradeInPlaceFailureKeepsOriginals: a database that cannot be
// upgraded leaves every configured database where and as it was.
func TestUpgradeInPlaceFailureKeepsOriginals(t *testing.T) {
	root := t.TempDir()
	good, bad := filepath.Join(root, "wonderland.db"), filepath.Join(root, "assets.db")
	gameplayFixture(t, good, 45)
	db, err := sql.Open("sqlite3", bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE original(value TEXT)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err = UpgradeInPlace([]Target{{Path: good}, {Path: bad, Assets: true}}, "test"); err == nil {
		t.Fatal("invalid assets accepted")
	}
	if goldAt(t, good) != 45 {
		t.Fatal("original moved or changed")
	}
	if _, err = os.Stat(bad); err != nil {
		t.Fatal("assets database moved", err)
	}
	if _, err = os.Stat(filepath.Join(root, ".migrate-test")); !os.IsNotExist(err) {
		t.Fatal("staging left behind", err)
	}
	if _, err = os.Stat(filepath.Join(root, "backups")); !os.IsNotExist(err) {
		t.Fatal("backup created for a failed run", err)
	}
}
