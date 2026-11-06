package store

import (
	"context"
	"errors"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestNativeManufacturingBatchRefundRestartAndRollback(t *testing.T) {
	db, ref, f, items, path := manufacturingStoreFixture(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	var material game.Item
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		c.TentReturn = &game.Location{Map: 10019, X: 1, Y: 1}
		c.Bag = game.Inventory{}
		material = game.Item{ID: 100, Count: 50, Damage: 9}
		material.Metadata[0] = 37
		for i := 0; i < 8; i++ {
			c.Bag[i] = material
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.PlanID = 0
	f.Inputs = [5]assets.ManufacturingInput{{ItemID: 100, Count: 200}}
	f.Output.Count = 1
	next, job, removes, err := db.StartNativeManufacturing(ctx, ref, 1, f, 2, 0, now, items)
	if err != nil || len(removes) != 8 || job.Count != 2 || job.Batch != 2 || job.DueAt != now.Add(20*time.Second).UnixMilli() || !next.Bag[0].Empty() {
		t.Fatal(job, removes, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 101, Count: 50}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.CancelNativeManufacturing(ctx, ref, 1, items); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	if job, err = db.ManufacturingJob(ctx, ref); err != nil || job == nil {
		t.Fatal("failed cancellation lost job", err)
	}
	if err = db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	next, _, err = db.CancelNativeManufacturing(ctx, ref, 1, items)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if next.Bag[i] != material {
			t.Fatalf("refund metadata/count lost at %d: %+v", i, next.Bag[i])
		}
	}
	if job, err = db.ManufacturingJob(ctx, ref); err != nil || job != nil {
		t.Fatal(job, err)
	}
	if _, _, err = db.CancelNativeManufacturing(ctx, ref, 1, items); err == nil {
		t.Fatal("replayed refund")
	}
}

func TestNativeManufacturingSurfaceBatchAndOutputOverflow(t *testing.T) {
	db, ref, f, items, _ := manufacturingStoreFixture(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error { c.TentReturn = &game.Location{Map: 10019}; return nil }); err != nil {
		t.Fatal(err)
	}
	f.Inputs = [5]assets.ManufacturingInput{{ItemID: 100, Count: 1}}
	f.TentOutput = true
	if _, _, _, err := db.StartNativeManufacturing(ctx, ref, 1, f, 255, 0, now, items); !errors.Is(err, game.ErrInvalidItem) {
		t.Fatal("overflow", err)
	}
	if _, _, _, err := db.StartNativeManufacturing(ctx, ref, 1, f, 2, assets.TentSurfaceWallpaper, now, items); err != nil {
		t.Fatal(err)
	}
	next, job, adds, err := db.CompleteManufacturing(ctx, ref, now.Add(time.Hour), items)
	if err != nil || job.Count != 4 || len(adds) != 1 || next.Bag[adds[0].Slot-1].Count != 3 {
		t.Fatal(job, adds, err)
	}
	tent, err := db.Tent(ctx, ref.ID)
	if err != nil || tent.Wallpaper != 500 || len(tent.Items) != 1 {
		t.Fatal(tent, err)
	}
	// Applying one surface with no spare copies must not require a bag grant of zero.
	f.Output.Count = 1
	if _, _, _, err = db.StartNativeManufacturing(ctx, ref, 1, f, 1, assets.TentSurfaceFloor, now, items); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = db.CompleteManufacturing(ctx, ref, now.Add(time.Hour), items); err != nil {
		t.Fatal(err)
	}
}
