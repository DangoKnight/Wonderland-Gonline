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

func TestCopyIncludesCommittedWALAndPreservesSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.db")
	destination := filepath.Join(root, "copy.db")
	s, err := store.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.Register(context.Background(), "tester", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: a.CharacterID(1), Name: "Tester", Slot: 1, Level: 1, Gold: 45}
	if err = s.CreateCharacter(context.Background(), a, c); err != nil {
		t.Fatal(err)
	}
	if err = Copy(source, destination, false); err != nil {
		t.Fatal(err)
	}
	copied, err := store.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	chars, err := copied.Characters(context.Background(), a.ID)
	if err != nil || len(chars) != 1 || chars[0].Gold != 45 {
		t.Fatal(chars, err)
	}
	if err = copied.UpdateCharacter(context.Background(), a.ID, c.ID, func(c *game.Character) error { c.Gold = 9; return nil }); err != nil {
		t.Fatal(err)
	}
	chars, err = s.Characters(context.Background(), a.ID)
	if err != nil || chars[0].Gold != 45 {
		t.Fatal("source modified", chars, err)
	}
	if err = Copy(source, destination, false); err == nil {
		t.Fatal("overwrote destination")
	}
	if err = Copy(source, source, false); err == nil {
		t.Fatal("accepted source as destination")
	}
}
func TestCopyFailureRemovesOnlyNewDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "bad.db")
	destination := filepath.Join(root, "copy.db")
	db, err := sql.Open("sqlite3", source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE original(value TEXT); INSERT INTO original VALUES('keep')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = Copy(source, destination, true); err == nil {
		t.Fatal("invalid assets accepted")
	}
	if _, err = os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("failed output retained", err)
	}
	db, err = sql.Open("sqlite3", source)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err = db.QueryRow("SELECT value FROM original").Scan(&value); err != nil || value != "keep" {
		t.Fatal("source damaged", value, err)
	}
}
