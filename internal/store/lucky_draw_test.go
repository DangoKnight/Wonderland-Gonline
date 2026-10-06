package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
)

func TestLuckyDrawDurableLimitPerCharacterAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draw.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	account, err := db.Register(ctx, "drawer", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range []byte{1, 2} {
		if err := db.CreateCharacter(ctx, account, game.Character{ID: account.CharacterID(slot), Slot: slot, Name: []string{"Alice", "Bobby"}[slot-1], Level: 1, HP: 1, MaxHP: 1}); err != nil {
			t.Fatal(err)
		}
	}
	today := time.Date(2026, 10, 2, 23, 59, 59, 0, time.UTC)
	ref := CharacterRef{Account: account.ID, ID: account.CharacterID(1)}
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := db.DrawLucky(ctx, ref, 100, 2, 50, today, nil)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else if !errors.Is(err, ErrLuckyDrawLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if successes != 3 {
		t.Fatal(successes)
	}
	other := CharacterRef{Account: account.ID, ID: account.CharacterID(2)}
	if next, _, err := db.DrawLucky(ctx, other, 100, 1, 50, today, nil); err != nil || next.LuckyDraw.Used != 1 {
		t.Fatal(next, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err := db.DrawLucky(ctx, ref, 100, 2, 50, today, nil); !errors.Is(err, ErrLuckyDrawLimit) {
		t.Fatal("restart reset allowance", err)
	}
	next, _, err := db.DrawLucky(ctx, ref, 100, 2, 50, today.Add(time.Second), nil)
	if err != nil || next.LuckyDraw.Used != 1 || next.LuckyDraw.Day != "2026-10-03" || next.Bag[0].Count != 8 {
		t.Fatal(next, err)
	}
	if _, _, err := db.DrawLucky(ctx, CharacterRef{Account: account.ID + 1, ID: ref.ID}, 100, 2, 50, today, nil); err == nil {
		t.Fatal("foreign character")
	}
}

func TestLuckyDrawFailedGrantAndSaveRollBackUsage(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	now := time.Now()
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 101, Count: 50}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.DrawLucky(ctx, refs[0], 100, 1, 50, now, nil); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	saved, err := db.Characters(ctx, refs[0].Account)
	if err != nil || saved[0].LuckyDraw.Used != 0 {
		t.Fatal(saved, err)
	}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`CREATE TRIGGER fail_lucky_save BEFORE UPDATE ON character_state BEGIN SELECT RAISE(ABORT, 'save failed'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.DrawLucky(ctx, refs[0], 100, 1, 50, now, nil); err == nil {
		t.Fatal("save failure succeeded")
	}
	saved, err = db.Characters(ctx, refs[0].Account)
	if err != nil || saved[0].LuckyDraw.Used != 0 || !saved[0].Bag[0].Empty() {
		t.Fatal(saved, err)
	}
}
