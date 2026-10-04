package store

import (
	"context"
	"errors"
	"testing"
	"time"
	"wonderland-go/internal/game"
)

func TestScriptedPropV14UpgradeFrameAndReopen(t *testing.T) {
	db, _, _, _, path := manufacturingStoreFixture(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	if err := db.SetMapPropState(ctx, 12000, 9, 1, now, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("ALTER TABLE map_props DROP COLUMN frame"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("PRAGMA user_version=14"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.ActiveMapProps(ctx, 12000, now)
	if err != nil || len(rows) != 1 || rows[0].Frame != 1 {
		t.Fatal("old implicit broken frame lost", rows, err)
	}
	if err = db.SetMapPropState(ctx, 12000, 9, 0, now, time.Minute); err != nil {
		t.Fatal(err)
	}
	rows, err = db.ActiveMapProps(ctx, 12000, now)
	if err != nil || len(rows) != 1 || rows[0].Frame != 0 {
		t.Fatal("explicit frame lost", rows, err)
	}
	expired, err := db.ExpireMapProps(ctx, now.Add(time.Minute))
	if err != nil || len(expired) != 1 {
		t.Fatal(expired, err)
	}
	expired, err = db.ExpireMapProps(ctx, now.Add(time.Minute))
	if err != nil || len(expired) != 0 {
		t.Fatal("duplicate expiry", err)
	}
}

func TestStarterPackWholeGrantRollbackAndOwnership(t *testing.T) {
	db, ref, _, _, _ := manufacturingStoreFixture(t)
	ctx := context.Background()
	items := map[uint16]game.ItemDefinition{500: {ID: 500, Type: 23}, 501: {ID: 501, Type: 23}}
	grants := []game.StarterGrant{{ID: 500, Count: 1}, {ID: 501, Count: 1}}
	if _, err := db.db.Exec("CREATE TRIGGER reject_starter_audit BEFORE INSERT ON audit BEGIN SELECT RAISE(ABORT,'audit failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.GrantStarterPack(ctx, ref, grants, items); err == nil {
		t.Fatal("audit failure did not rollback")
	}
	chars, err := db.Characters(ctx, ref.Account)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range chars[0].Bag {
		if item.ID == 500 || item.ID == 501 {
			t.Fatal("rolled back starter grant persisted")
		}
	}
	if _, err = db.db.Exec("DROP TRIGGER reject_starter_audit"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.GrantStarterPack(ctx, ref, grants, items); err != nil {
		t.Fatal(err)
	}
	// Two individually valid outputs cannot partially grant into one empty slot.
	if err = db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 100, Count: 50}
		}
		c.Bag[0] = game.Item{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.GrantStarterPack(ctx, ref, grants, items); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal("expected overflow", err)
	}
	chars, err = db.Characters(ctx, ref.Account)
	if err != nil || !chars[0].Bag[0].Empty() {
		t.Fatal("partial pack persisted", err)
	}
	if err = db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error { c.Level = 2; c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.GrantStarterPack(ctx, ref, grants, items); err != nil {
		t.Fatal("manual grant to higher level failed", err)
	}
	if _, _, err = db.GrantStarterPack(ctx, CharacterRef{Account: ref.Account + 1, ID: ref.ID}, grants, items); err == nil {
		t.Fatal("foreign grant accepted")
	}
}
