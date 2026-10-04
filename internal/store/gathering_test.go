package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wonderland-go/internal/game"
)

func TestWaterGatheringAtomicCooldownAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gather.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	ctx := context.Background()
	a, err := db.Register(ctx, "gatherer", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	for slot := byte(1); slot <= 2; slot++ {
		if err := db.CreateCharacter(ctx, a, game.Character{ID: a.CharacterID(slot), Slot: slot, Name: []string{"Alice", "Bobby"}[slot-1], Level: 1}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	ref := CharacterRef{Account: a.ID, ID: a.CharacterID(1)}
	eligible := func(*game.Character) bool { return true }
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := db.GatherWater(ctx, ref, 11009, 60001, 50, now, eligible, nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrGatheringUnavailable) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal(successes)
	}
	chars, err := db.Characters(ctx, a.ID)
	if err != nil || chars[0].Bag[0].ID != 60001 || chars[0].Bag[0].Count != 1 || chars[0].Quests[11009].State != game.InProgress || chars[0].Quests[11009].Step != 1 || !chars[0].EventTimers[11009].Equal(now.Add(180*time.Second)) {
		t.Fatal(chars, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.GatherWater(ctx, ref, 11009, 60001, 50, now.Add(179*time.Second), eligible, nil); !errors.Is(err, ErrGatheringUnavailable) {
		t.Fatal("restart reset cooldown", err)
	}
	if _, _, err := db.GatherWater(ctx, ref, 11009, 60001, 50, now.Add(-time.Second), eligible, nil); !errors.Is(err, ErrGatheringUnavailable) {
		t.Fatal("clock rollback reset cooldown", err)
	}
	if next, _, err := db.GatherWater(ctx, ref, 11009, 60001, 50, now.Add(180*time.Second), eligible, nil); err != nil || next.Bag[0].Count != 2 {
		t.Fatal(next, err)
	}
	for _, other := range []struct {
		ref         CharacterRef
		timer, item uint16
	}{{CharacterRef{Account: a.ID, ID: a.CharacterID(2)}, 11009, 60001}, {ref, 11016, 60001}} {
		if _, _, err := db.GatherWater(ctx, other.ref, other.timer, other.item, 50, now, eligible, nil); err != nil {
			t.Fatal("independent timer denied", err)
		}
	}
}

func TestWaterGatheringFailureRollbackAndOwnership(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	now := time.Now()
	yes := func(*game.Character) bool { return true }
	no := func(*game.Character) bool { return false }
	if _, _, err := db.GatherWater(ctx, refs[0], 11009, 60001, 50, now, no, nil); !errors.Is(err, ErrGatheringUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := db.GatherWater(ctx, refs[0], 11009, 60002, 50, now, yes, nil); !errors.Is(err, ErrGatheringUnavailable) {
		t.Fatal("wrong pool reward", err)
	}
	foreign := refs[0]
	foreign.Account = refs[1].Account
	if _, _, err := db.GatherWater(ctx, foreign, 11009, 60001, 50, now, yes, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 60001, Count: 1}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.GatherWater(ctx, refs[0], 11009, 60001, 50, now, yes, nil); !errors.Is(err, ErrGatheringUnavailable) {
		t.Fatal("native free-slot requirement ignored", err)
	}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("CREATE TRIGGER fail_gather BEFORE UPDATE ON character_state BEGIN SELECT RAISE(ABORT,'injected failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, adds, err := db.GatherWater(ctx, refs[0], 11009, 60001, 50, now, yes, nil); err == nil || adds != nil {
		t.Fatal(adds, err)
	}
	chars, err := db.Characters(ctx, refs[0].Account)
	if err != nil || !chars[0].Bag[0].Empty() || len(chars[0].EventTimers) != 0 || len(chars[0].Quests) != 0 {
		t.Fatal("partial save", chars, err)
	}
	if _, err := db.db.Exec("DROP TRIGGER fail_gather"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.GatherWater(ctx, refs[0], 11009, 60001, 50, now, yes, nil); err != nil {
		t.Fatal(err)
	}
}
