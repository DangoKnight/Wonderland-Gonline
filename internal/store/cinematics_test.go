package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestCinematicCompletionMigrationRestartAndOwnership(t *testing.T) {
	s, a, ref, path := mallStoreFixture(t)
	ctx := context.Background()
	if _, err := s.db.Exec("DROP TABLE character_cinematics; PRAGMA user_version=18"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	done, err := s.CinematicCompleted(ctx, ref, "arrival")
	if err != nil || done {
		t.Fatal("new completion", done, err)
	}
	wrong := CharacterRef{Account: a.ID + 1, ID: ref.ID}
	if _, err := s.CinematicCompleted(ctx, wrong, "arrival"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("read ownership", err)
	}
	if err := s.CompleteCinematic(ctx, wrong, "arrival", time.Now()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("write ownership", err)
	}
	if err := s.CompleteCinematic(ctx, ref, "arrival", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteCinematic(ctx, ref, "arrival", time.Now()); err != nil {
		t.Fatal("duplicate", err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if done, err := s.CinematicCompleted(ctx, ref, "arrival"); err != nil || !done {
		t.Fatal("completion lost after restart", err)
	}
	chars, err := s.Characters(ctx, a.ID)
	if err != nil || len(chars) != 1 || len(chars[0].Quests) != 0 || chars[0].Name != "Shopper" {
		t.Fatal("migration or receipt changed gameplay", err)
	}
	failed, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.CompleteCinematic(failed, ref, "interrupted", time.Now()); err == nil {
		t.Fatal("canceled write succeeded")
	}
	if done, err := s.CinematicCompleted(ctx, ref, "interrupted"); err != nil || done {
		t.Fatal("failed write persisted", err)
	}
}
