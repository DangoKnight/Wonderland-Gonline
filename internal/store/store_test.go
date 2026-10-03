package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"wonderland-go/internal/game"
)

func TestAccountsPersistAndBan(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.Register(ctx, "Alice", "secret12", "a@example.test")
	if e != nil {
		t.Fatal(e)
	}
	if a.UserID() != 10001 || a.CharacterID(2) != 4510001 {
		t.Fatal(a)
	}
	if _, e = s.Register(ctx, "ALICE", "secret12", ""); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, "alice", "wrong"); !errors.Is(e, ErrCredentials) {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.Authenticate(ctx, "ALICE", "secret12"); e != nil {
		t.Fatal(e)
	}
	if e = s.SetBanned(ctx, a.ID, true); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, "alice", "secret12"); !errors.Is(e, ErrCredentials) {
		t.Fatal(e)
	}
}
func TestConcurrentRegistration(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Register(context.Background(), "same", "secret", "")
			if e != nil && !errors.Is(e, ErrConflict) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	a, e := s.Accounts(context.Background())
	if e != nil || len(a) != 1 {
		t.Fatalf("%v %v", a, e)
	}
}
func TestCharacterMutationRollback(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	a, e := s.Register(ctx, "owner", "secret", "")
	if e != nil {
		t.Fatal(e)
	}
	c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: "Player", Level: 1, HP: 100, MaxHP: 100, SP: 50, MaxSP: 50}
	if e = s.CreateCharacter(ctx, a, c); e != nil {
		t.Fatal(e)
	}
	stop := errors.New("abort")
	if e = s.UpdateCharacter(ctx, a.ID, c.ID, func(c *game.Character) error { c.Gold = 123; return stop }); !errors.Is(e, stop) {
		t.Fatal(e)
	}
	list, e := s.Characters(ctx, a.ID)
	if e != nil || len(list) != 1 || list[0].Gold != 0 {
		t.Fatal(list, e)
	}
	if e = s.UpdateCharacter(ctx, a.ID+1, c.ID, func(*game.Character) error { return nil }); e == nil {
		t.Fatal("cross-account mutation")
	}
}

func TestDeletionCodeAndCharacterOwnership(t *testing.T) {
	ctx := context.Background()
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	a, e := s.Register(ctx, "owner", "password", "")
	if e != nil {
		t.Fatal(e)
	}
	other, e := s.Register(ctx, "other", "password", "")
	if e != nil {
		t.Fatal(e)
	}
	c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: "Player", Level: 1, HP: 100, MaxHP: 100}
	if e = s.CreateCharacterWithCode(ctx, a, c, "delete-me"); e != nil {
		t.Fatal(e)
	}
	if has, e := s.HasDeletionCode(ctx, a.ID); e != nil || !has {
		t.Fatal(has, e)
	}
	if e = s.DeleteCharacter(ctx, a, 1, "wrong-code"); !errors.Is(e, ErrCredentials) {
		t.Fatal(e)
	}
	if e = s.DeleteCharacter(ctx, other, 1, "delete-me"); e != nil {
		t.Fatal(e)
	}
	chars, e := s.Characters(ctx, a.ID)
	if e != nil || len(chars) != 1 {
		t.Fatal("other account deleted character")
	}
	if e = s.DeleteCharacter(ctx, a, 1, "delete-me"); e != nil {
		t.Fatal(e)
	}
	chars, e = s.Characters(ctx, a.ID)
	if e != nil || len(chars) != 0 {
		t.Fatal(chars, e)
	}
	if has, e := s.HasDeletionCode(ctx, a.ID); e != nil || has {
		t.Fatal("last-character deletion retained code", e)
	}
}

func TestMigrationPreservesVersionOneAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	if _, e = s.Register(ctx, "owner", "password", ""); e != nil {
		t.Fatal(e)
	}
	// Recreate the previous schema shape in an isolated fixture.
	if _, e = s.db.Exec("DROP TABLE account_security; PRAGMA user_version=1;"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.Authenticate(ctx, "owner", "password"); e != nil {
		t.Fatal(e)
	}
	var version int
	if e = s.db.QueryRow("PRAGMA user_version").Scan(&version); e != nil || version != schemaVersion {
		t.Fatal(version, e)
	}
	// A v2 accounts table has no GM column; migration adds it without granting GM.
	if _, e = s.db.Exec("ALTER TABLE accounts DROP COLUMN gm_level; PRAGMA user_version=2;"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	if s, e = Open(path); e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if a, e := s.Authenticate(ctx, "owner", "password"); e != nil || a.GMLevel != 0 {
		t.Fatal(a, e)
	}
}

func TestGMLevelIsAuditedAndRevocable(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	a, e := s.Register(ctx, "admin", "password", "")
	if e != nil || a.GMLevel != 0 {
		t.Fatal("registration granted GM", a, e)
	}
	if e = s.SetGMLevel(ctx, a.ID, 1); e != nil {
		t.Fatal(e)
	}
	if a, e = s.Authenticate(ctx, "admin", "password"); e != nil || a.GMLevel != 1 {
		t.Fatal(a, e)
	}
	if e = s.SetGMLevel(ctx, a.ID, 0); e != nil {
		t.Fatal(e)
	}
	accounts, e := s.Accounts(ctx)
	if e != nil || accounts[0].GMLevel != 0 {
		t.Fatal(accounts, e)
	}
	var audits int
	if e = s.db.QueryRow("SELECT count(*) FROM audit WHERE action LIKE 'gm_level=%'").Scan(&audits); e != nil || audits != 2 {
		t.Fatal(audits, e)
	}
	if e = s.SetGMLevel(ctx, 999, 1); e == nil {
		t.Fatal("granted GM to a missing account")
	}
}
