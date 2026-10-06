package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestAccountDeleteAndPasswordReset(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	a, err := s.Register(ctx, "tester", "password", "test@example.test")
	if err != nil {
		t.Fatal(err)
	}
	for slot := byte(1); slot <= 2; slot++ {
		c := game.Character{ID: a.CharacterID(slot), Slot: slot, Name: []string{"First", "Second"}[slot-1], Level: 1, HP: 10, MaxHP: 10}
		if err := s.CreateCharacterWithCode(ctx, a, c, "delete-me"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetGMLevel(ctx, a.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, a.ID, "new-secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, "tester", "password"); !errors.Is(err, ErrCredentials) {
		t.Fatal("old password accepted", err)
	}
	if got, err := s.Authenticate(ctx, "tester", "new-secret"); err != nil || got.GMLevel != 1 || got.Email != a.Email {
		t.Fatal("reset changed account attributes", got, err)
	}
	if has, err := s.HasDeletionCode(ctx, a.ID); err != nil || !has {
		t.Fatal("reset removed deletion code", err)
	}
	for _, password := range []string{"", "abc", strings.Repeat("x", 15), "pässword", "pass\nword"} {
		if err := s.ResetPassword(ctx, a.ID, password); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	if err := s.ResetPassword(ctx, 999, "password"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("missing account reset", err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, "tester", "new-secret"); err != nil {
		t.Fatal("reset not persisted", err)
	}
	if err := s.DeleteAccount(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"accounts", "characters", "account_security"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("orphaned account data", table, count, err)
		}
	}
	if err := s.DeleteAccount(ctx, a.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("repeated deletion", err)
	}
	var resets, deletions int
	s.db.QueryRow("SELECT count(*) FROM audit WHERE action='password_reset'").Scan(&resets)
	s.db.QueryRow("SELECT count(*) FROM audit WHERE action='account_delete'").Scan(&deletions)
	if resets != 1 || deletions != 1 {
		t.Fatal("audit count", resets, deletions)
	}
	var secrets int
	s.db.QueryRow("SELECT count(*) FROM audit WHERE action LIKE '%secret%' OR action LIKE '%pbkdf2%'").Scan(&secrets)
	if secrets != 0 {
		t.Fatal("audit contains credential")
	}
	replacement, err := s.Register(ctx, "tester", "password", "")
	if err != nil || replacement.ID == a.ID {
		t.Fatal("deleted identity reused", replacement, err)
	}
}

func TestAccountAdminAuditFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err := s.Register(ctx, "tester", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: "First", Level: 1, HP: 10, MaxHP: 10}
	if err := s.CreateCharacterWithCode(ctx, a, c, "delete-me"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER fail_audit BEFORE INSERT ON audit BEGIN SELECT RAISE(ABORT,'audit unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetPassword(ctx, a.ID, "new-secret"); err == nil {
		t.Fatal("audit failure ignored")
	}
	if _, err := s.Authenticate(ctx, "tester", "password"); err != nil {
		t.Fatal("failed reset changed password", err)
	}
	if err := s.DeleteAccount(ctx, a.ID); err == nil {
		t.Fatal("delete audit failure ignored")
	}
	if chars, err := s.Characters(ctx, a.ID); err != nil || len(chars) != 1 {
		t.Fatal("failed delete removed characters", err)
	}
	if has, err := s.HasDeletionCode(ctx, a.ID); err != nil || !has {
		t.Fatal("failed delete removed security", err)
	}
	if _, err := s.Authenticate(ctx, "tester", "password"); err != nil {
		t.Fatal("failed delete removed account", err)
	}
}
