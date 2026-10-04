package store

import (
	"context"
	"path/filepath"
	"testing"
	"wonderland-go/internal/game"
)

func TestNativeMetadataPreservingUpgradeAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, err := s.Register(ctx, "native", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: "Native", Level: 1, Map: 10017, HP: 100, MaxHP: 100, SP: 25, MaxSP: 25, Gold: 123, X: 44, Y: 55}
	c.Bag[7] = game.Item{ID: 100, Count: 9}
	c.Bag[7].Metadata[2] = 77
	if err = s.CreateCharacter(ctx, a, c); err != nil {
		t.Fatal(err)
	}
	// Simulate the actual v11 schema by removing the new columns.
	for _, stmt := range []string{"ALTER TABLE character_state DROP COLUMN title", "ALTER TABLE character_state DROP COLUMN reborn_job", "PRAGMA user_version=11"} {
		if _, err = s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatal(version, err)
	}
	chars, err := s.Characters(ctx, a.ID)
	if err != nil || len(chars) != 1 || CharacterVersion(chars[0]) != CharacterVersion(c) {
		t.Fatal("upgrade changed existing state", chars, err)
	}
	if err = s.UpdateCharacter(ctx, a.ID, c.ID, func(next *game.Character) error { next.Title = 65535; next.RebornJob = 6; return nil }); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	chars, err = s.Characters(ctx, a.ID)
	c.Title, c.RebornJob = 65535, 6
	if err != nil || len(chars) != 1 || CharacterVersion(chars[0]) != CharacterVersion(c) {
		t.Fatal("native metadata lost on reopen", chars, err)
	}
}
