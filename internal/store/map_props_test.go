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

func TestMapPropAtomicClaimsRecoveryAndFullInventory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "props.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	ctx := context.Background()
	a, err := db.Register(ctx, "propuser", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	for slot := byte(1); slot <= 2; slot++ {
		if err = db.CreateCharacter(ctx, a, game.Character{ID: a.CharacterID(slot), Slot: slot, Name: []string{"Alice", "Bobby"}[slot-1], Level: 1, Map: 12000}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Unix(1000, 0)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := db.ClaimMapProp(ctx, CharacterRef{a.ID, a.CharacterID(byte(i%2 + 1))}, 12000, 10, game.Item{ID: 1, Count: 1}, 50, now, time.Minute, nil)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	claims := 0
	for err := range errs {
		if err == nil {
			claims++
		} else if !errors.Is(err, ErrPropEmpty) {
			t.Fatal(err)
		}
	}
	if claims != 1 {
		t.Fatal("duplicate shared reward", claims)
	}
	db.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := db.ActiveMapProps(ctx, 12000, now.Add(30*time.Second)); err != nil || len(rows) != 1 {
		t.Fatal("recovery", rows, err)
	}
	if rows, err := db.ExpireMapProps(ctx, now.Add(time.Minute)); err != nil || len(rows) != 1 {
		t.Fatal("expiry", rows, err)
	}
	if rows, err := db.ExpireMapProps(ctx, now.Add(time.Minute)); err != nil || len(rows) != 0 {
		t.Fatal("repeated expiry", rows, err)
	}
	ref := CharacterRef{a.ID, a.CharacterID(1)}
	if err = db.UpdateCharacter(ctx, a.ID, ref.ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 2, Count: 50}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.ClaimMapProp(ctx, ref, 12000, 10, game.Item{ID: 1, Count: 1}, 50, now.Add(time.Minute), time.Minute, nil); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	if rows, _ := db.ActiveMapProps(ctx, 12000, now.Add(time.Minute)); len(rows) != 0 {
		t.Fatal("full inventory broke node", rows)
	}
	if _, err = db.db.Exec("CREATE TRIGGER reject_prop BEFORE INSERT ON map_props BEGIN SELECT RAISE(ABORT,'reject'); END"); err != nil {
		t.Fatal(err)
	}
	if err = db.UpdateCharacter(ctx, a.ID, ref.ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.ClaimMapProp(ctx, ref, 12000, 10, game.Item{ID: 1, Count: 1}, 50, now.Add(time.Minute), time.Minute, nil); err == nil {
		t.Fatal("failed cooldown write accepted")
	}
	chars, _ := db.Characters(ctx, a.ID)
	if !chars[0].Bag[0].Empty() {
		t.Fatal("reward survived rollback")
	}
}
