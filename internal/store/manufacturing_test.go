package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func manufacturingStoreFixture(t *testing.T) (*Store, CharacterRef, assets.ManufacturingFormula, map[uint16]game.ItemDefinition, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manufacturing.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	account, err := db.Register(ctx, "builder", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	c := game.Character{ID: account.CharacterID(1), Slot: 1, Name: "Builder", Level: 1, HP: 1, MaxHP: 1, Map: game.TentMapID}
	for i := 0; i < 4; i++ {
		c.Bag[i] = game.Item{ID: uint16(100 + i), Count: 5}
	}
	c.Bag[4] = game.Item{ID: 200, Count: 1}
	if err = db.CreateCharacter(ctx, account, c); err != nil {
		t.Fatal(err)
	}
	ref := CharacterRef{Account: account.ID, ID: c.ID}
	if _, err = db.EnsureTent(ctx, ref, 39062, 39064, []TentItem{{ItemID: 300}}); err != nil {
		t.Fatal(err)
	}
	items := map[uint16]game.ItemDefinition{}
	for _, id := range []uint16{100, 101, 102, 103, 200, 300, 500} {
		items[id] = game.ItemDefinition{ID: id, Type: 23}
	}
	formula := assets.ManufacturingFormula{ID: 1, Output: assets.ManufacturingInput{ItemID: 500, Count: 2}, PlanID: 200, ToolID: 300, DurationSeconds: 10, Inputs: [5]assets.ManufacturingInput{{ItemID: 100, Count: 1}, {ItemID: 101, Count: 1}, {ItemID: 100, Count: 2}, {ItemID: 102, Count: 1}, {ItemID: 103, Count: 1}}}
	return db, ref, formula, items, path
}
func TestManufacturingDurablePauseResumeAndExactlyOnce(t *testing.T) {
	db, ref, f, items, path := manufacturingStoreFixture(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	next, job, _, err := db.StartManufacturing(ctx, ref, 0, f, now, items)
	if err != nil {
		t.Fatal(err)
	}
	if next.Bag[0].Count != 2 || next.Bag[4].Count != 1 {
		t.Fatal("materials or plan", next.Bag)
	}
	if _, _, _, err = db.StartManufacturing(ctx, ref, 0, f, now, items); !errors.Is(err, ErrManufacturingBusy) {
		t.Fatal(err)
	}
	if _, _, _, err = db.CompleteManufacturing(ctx, ref, now, items); !errors.Is(err, ErrManufacturingNotDue) {
		t.Fatal(err)
	}
	job, err = db.PauseManufacturing(ctx, ref, 0, true, now.Add(5*time.Second))
	if err != nil || job.RemainingMillis != 5000 {
		t.Fatal(job, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	job, err = db.PauseManufacturing(ctx, ref, 0, false, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.UnixMilli(job.DueAt)
	again, err := db.PauseManufacturing(ctx, ref, 0, false, now.Add(time.Hour+time.Second))
	if err != nil || again.DueAt != job.DueAt {
		t.Fatal("continue extended deadline", again, err)
	}
	if _, err = db.db.Exec("CREATE TRIGGER fail_manufacture_delivery BEFORE DELETE ON manufacture_jobs BEGIN SELECT RAISE(ABORT, 'forced manufacture failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = db.CompleteManufacturing(ctx, ref, deadline, items); err == nil {
		t.Fatal("SQL failure accepted")
	}
	saved, _ := db.Characters(ctx, ref.Account)
	for _, item := range saved[0].Bag {
		if item.ID == 500 {
			t.Fatal("delivery survived rollback")
		}
	}
	if _, err = db.db.Exec("DROP TRIGGER fail_manufacture_delivery"); err != nil {
		t.Fatal(err)
	}
	next, _, adds, err := db.CompleteManufacturing(ctx, ref, deadline, items)
	if err != nil || len(adds) != 1 || next.Bag[adds[0].Slot-1].Count != 2 {
		t.Fatal(adds, err)
	}
	if _, _, _, err = db.CompleteManufacturing(ctx, ref, deadline, items); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("repeated completion", err)
	}
}
func TestManufacturingCapacityRetainsOutputAndTentDelivery(t *testing.T) {
	db, ref, f, items, _ := manufacturingStoreFixture(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	if _, _, _, err := db.StartManufacturing(ctx, ref, 0, f, now, items); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 100, Count: 50}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := db.CompleteManufacturing(ctx, ref, now.Add(time.Hour), items); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	if job, err := db.ManufacturingJob(ctx, ref); err != nil || job == nil {
		t.Fatal("full bag lost output", err)
	}
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error { c.Bag[0] = game.Item{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := db.CompleteManufacturing(ctx, ref, now.Add(time.Hour), items); err != nil {
		t.Fatal(err)
	}
	// A separate job delivers every furniture unit to its owner's tent.
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		c.Bag = game.Inventory{}
		for i := 0; i < 4; i++ {
			c.Bag[i] = game.Item{ID: uint16(100 + i), Count: 5}
		}
		c.Bag[4] = game.Item{ID: 200, Count: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.TentOutput = true
	if _, _, _, err := db.StartManufacturing(ctx, CharacterRef{Account: ref.Account + 1, ID: ref.ID}, 0, f, now, items); err == nil {
		t.Fatal("wrong account accepted")
	}
	if _, _, _, err := db.StartManufacturing(ctx, ref, 0, f, now, items); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := db.CompleteManufacturing(ctx, ref, now.Add(time.Hour), items); err != nil {
		t.Fatal(err)
	}
	tent, err := db.Tent(ctx, ref.ID)
	if err != nil || len(tent.Items) != 3 || tent.Items[1].ItemID != 500 || tent.Items[2].ItemID != 500 {
		t.Fatal(tent, err)
	}
}
