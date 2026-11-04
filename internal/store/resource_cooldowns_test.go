package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
)

func TestSharedResourceClocksAcrossPlayersAndConnections(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	for _, ref := range refs {
		if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
			c.Map = 60000
			c.EventTimers = map[uint16]time.Time{11020: now.Add(time.Hour)}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	var path string
	if err := db.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		t.Fatal(err)
	}
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	stores := []*Store{db, other}
	node := ResourceKey{Kind: ResourceCollectionActor, MapID: 60000, NodeID: 33}
	items := map[uint16]game.ItemDefinition{40012: {ID: 40012, Type: 10}}
	eligible := func(*game.Character) bool { return true }
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := stores[i%2].GatherResources(ctx, refs[i%2], node, 11020, 30*time.Second, map[uint16]int{40012: 1}, now, eligible, items)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	wins := 0
	for err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrGatheringUnavailable) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatal("shared resource had multiple winners", wins)
	}
	sibling := node
	sibling.NodeID++
	if _, _, err := other.GatherResources(ctx, refs[1], sibling, 11020, 90*time.Second, map[uint16]int{40012: 1}, now, eligible, items); err != nil {
		t.Fatal("same legacy timer blocked a different spot", err)
	}
	for _, test := range []struct {
		key     ResourceKey
		seconds int
	}{{node, 30}, {sibling, 90}} {
		expiry, err := db.ResourceExpiry(ctx, test.key)
		if err != nil || !expiry.Equal(now.Add(time.Duration(test.seconds)*time.Second)) {
			t.Fatal("source lost its own duration", test, expiry, err)
		}
	}
	// A ground source with the same numeric identity is an independent namespace.
	ground := node
	ground.Kind = ResourceGroundItem
	if _, err := db.ClaimGroundItem(ctx, refs[0], ground, game.Item{ID: 40012, Count: 1}, 50, now, 10*time.Second, items); err != nil {
		t.Fatal(err)
	}
	if _, err := other.ClaimGroundItem(ctx, refs[1], ground, game.Item{ID: 40012, Count: 1}, 50, now.Add(9*time.Second), 10*time.Second, items); !errors.Is(err, ErrResourceUnavailable) {
		t.Fatal("second player bypassed ground cooldown", err)
	}
	if _, err := other.ClaimGroundItem(ctx, refs[1], ground, game.Item{ID: 40012, Count: 1}, 50, now.Add(10*time.Second), 10*time.Second, items); err != nil {
		t.Fatal("ground boundary did not expire", err)
	}
	if _, _, err := db.GatherResources(ctx, refs[0], node, 11020, 30*time.Second, map[uint16]int{40012: 1}, now.Add(30*time.Second), eligible, items); err != nil {
		t.Fatal("short spot did not expire", err)
	}
	if _, _, err := db.GatherResources(ctx, refs[0], sibling, 11020, 90*time.Second, map[uint16]int{40012: 1}, now.Add(30*time.Second), eligible, items); !errors.Is(err, ErrGatheringUnavailable) {
		t.Fatal("short spot reset the long spot", err)
	}
}

func TestGroundClaimRollbackAndReopen(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	node := ResourceKey{Kind: ResourceGroundItem, MapID: 60000, NodeID: 5}
	items := map[uint16]game.ItemDefinition{40012: {ID: 40012, Type: 10}}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		c.Map = node.MapID
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 40012, Count: 50}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimGroundItem(ctx, refs[0], node, game.Item{ID: 40012, Count: 1}, 50, now, time.Minute, items); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	if expiry, err := db.ResourceExpiry(ctx, node); err != nil || !expiry.IsZero() {
		t.Fatal("failed grant consumed resource", expiry, err)
	}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ClaimGroundItem(ctx, refs[0], node, game.Item{ID: 40012, Count: 1}, 50, now, time.Minute, items); err != nil {
		t.Fatal(err)
	}
	var path string
	if err := db.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	expiry, err := reopened.ResourceExpiry(ctx, node)
	if err != nil || !expiry.Equal(now.Add(time.Minute)) {
		t.Fatal("restart lost shared ground timer", expiry, err)
	}
	if _, err := reopened.ClaimGroundItem(ctx, refs[0], node, game.Item{ID: 40012, Count: 1}, 50, now, time.Minute, items); !errors.Is(err, ErrResourceUnavailable) {
		t.Fatal("restart duplicated ground grant", err)
	}
}
