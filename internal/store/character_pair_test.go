package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"wonderland-gonline/internal/game"
)

func pairFixture(t *testing.T) (*Store, [2]CharacterRef) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "pair.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var refs [2]CharacterRef
	for i, name := range []string{"Alice", "Bobby"} {
		a, err := db.Register(context.Background(), name, "password", "")
		if err != nil {
			t.Fatal(err)
		}
		c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: name, Level: 1, HP: 1, MaxHP: 1, Gold: 100}
		if err := db.CreateCharacter(context.Background(), a, c); err != nil {
			t.Fatal(err)
		}
		refs[i] = CharacterRef{Account: a.ID, ID: c.ID}
	}
	return db, refs
}
func pairGold(t *testing.T, db *Store, refs [2]CharacterRef) [2]uint32 {
	t.Helper()
	var result [2]uint32
	for i, ref := range refs {
		chars, err := db.Characters(context.Background(), ref.Account)
		if err != nil || len(chars) != 1 {
			t.Fatal(err)
		}
		result[i] = chars[0].Gold
	}
	return result
}

func TestCharacterPairRollsBackSecondRowFailure(t *testing.T) {
	db, refs := pairFixture(t)
	statement := fmt.Sprintf("CREATE TRIGGER reject_second BEFORE UPDATE ON character_state WHEN NEW.character_id=%d BEGIN SELECT RAISE(ABORT,'injected second-row failure'); END", refs[1].ID)
	if _, err := db.db.Exec(statement); err != nil {
		t.Fatal(err)
	}
	err := db.UpdateCharacterPair(context.Background(), refs[0], refs[1], func(a, b *game.Character) error { a.Gold = 90; b.Gold = 110; return nil })
	if err == nil || pairGold(t, db, refs) != [2]uint32{100, 100} {
		t.Fatal("partial pair committed", err)
	}
}

func TestCharacterPairRejectsCallbackIdentityAndOwnership(t *testing.T) {
	db, refs := pairFixture(t)
	for _, mutate := range []func(*game.Character, *game.Character) error{
		func(a, b *game.Character) error { a.Gold = 90; return errors.New("abort") },
		func(a, b *game.Character) error { a.Gold = 90; b.Name = "Changed"; return nil },
		func(a, b *game.Character) error { a.Gold = 90; b.HP = 2; return nil },
	} {
		if err := db.UpdateCharacterPair(context.Background(), refs[0], refs[1], mutate); err == nil {
			t.Fatal("invalid callback accepted")
		}
		if pairGold(t, db, refs) != [2]uint32{100, 100} {
			t.Fatal("failed callback persisted a side")
		}
	}
	bad := refs[1]
	bad.Account = refs[0].Account
	if err := db.UpdateCharacterPair(context.Background(), refs[0], bad, func(a, b *game.Character) error { a.Gold = 0; b.Gold = 0; return nil }); err == nil {
		t.Fatal("foreign character changed")
	}
	if err := db.UpdateCharacterPair(context.Background(), refs[0], refs[0], func(a, b *game.Character) error { return nil }); err == nil {
		t.Fatal("self pair accepted")
	}
}

func TestCharacterPairConcurrentTransfers(t *testing.T) {
	db, refs := pairFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- db.UpdateCharacterPair(context.Background(), refs[0], refs[1], func(a, b *game.Character) error { a.Gold--; b.Gold++; return nil })
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if pairGold(t, db, refs) != [2]uint32{80, 120} {
		t.Fatal("concurrent pair lost gold")
	}
}
