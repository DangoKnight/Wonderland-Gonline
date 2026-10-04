package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wonderland-go/internal/game"
)

func TestFishingDeadlineOwnershipRollbackAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fishing.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	a, err := db.Register(ctx, "fisher", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: a.CharacterID(1), Slot: 1, Name: "Fisher", Level: 1, HP: 1, MaxHP: 1}
	c.Bag[0] = game.Item{ID: 37105, Count: 1}
	if err = db.CreateCharacter(ctx, a, c); err != nil {
		t.Fatal(err)
	}
	// Reopen a genuine v12 layout without the new table; retain the character.
	if _, err = db.db.Exec("DROP TABLE fishing_progress; PRAGMA user_version=12"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatal(version, err)
	}
	before, err := db.Characters(ctx, a.ID)
	if err != nil || !CharacterSnapshotsEqual(before[0], c) {
		t.Fatal("fishing upgrade changed character", err)
	}

	ref := CharacterRef{Account: a.ID, ID: c.ID}
	now := time.Unix(100, 0)
	interval := time.Minute
	_, due, err := db.MutateFishing(ctx, ref, 1, 37105, now, interval, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := db.MutateFishing(ctx, ref, 1, 37105, now.Add(time.Second), interval, false, nil)
	if err != nil || !again.Equal(due) {
		t.Fatal(again, err)
	}
	grant := func(c *game.Character) error { return c.Bag.Add(game.Item{ID: 44001}, 1, 50) }
	if _, _, err = db.MutateFishing(ctx, ref, 1, 37105, due.Add(-time.Millisecond), interval, true, grant); !errors.Is(err, ErrFishingNotDue) {
		t.Fatal(err)
	}
	fail := errors.New("delivery failed")
	if _, _, err = db.MutateFishing(ctx, ref, 1, 37105, due, interval, true, func(c *game.Character) error { c.Gold++; return fail }); !errors.Is(err, fail) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := db.MutateFishing(ctx, ref, 1, 37105, due, interval, true, grant)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrFishingNotDue) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err = db.MutateFishing(ctx, ref, 1, 37105, due, interval, true, grant); !errors.Is(err, ErrFishingNotDue) {
		t.Fatal(err)
	}
	chars, err := db.Characters(ctx, a.ID)
	if err != nil || chars[0].Gold != 0 || chars[0].Bag[1].Count != 1 {
		t.Fatal(chars, err)
	}
	if _, _, err = db.MutateFishing(ctx, CharacterRef{Account: a.ID + 1, ID: c.ID}, 1, 37105, due.Add(interval), interval, true, grant); err == nil {
		t.Fatal("wrong account accepted")
	}
	if _, err = db.db.Exec("CREATE TRIGGER fishing_save_failure BEFORE UPDATE ON fishing_progress BEGIN SELECT RAISE(ABORT, 'forced fishing failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.MutateFishing(ctx, ref, 1, 37105, due.Add(interval), interval, true, grant); err == nil {
		t.Fatal("SQL failure accepted")
	}
	chars, err = db.Characters(ctx, a.ID)
	if err != nil || chars[0].Bag[1].Count != 1 {
		t.Fatal("SQL failure retained catch", err)
	}
	if _, err = db.db.Exec("DROP TRIGGER fishing_save_failure"); err != nil {
		t.Fatal(err)
	}

	if err = db.UpdateCharacter(ctx, a.ID, c.ID, func(c *game.Character) error { c.Bag[0] = game.Item{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.MutateFishing(ctx, ref, 1, 37105, due.Add(interval), interval, true, grant); !errors.Is(err, game.ErrInvalidItem) {
		t.Fatal(err)
	}
}
