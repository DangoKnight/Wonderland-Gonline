package store

import (
	"context"
	"errors"
	"testing"
	"time"
	"wonderland-go/internal/game"
)

func TestMultiSlotLuckyDrawRollbackAndFootprintPersistence(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	items := map[uint16]game.ItemDefinition{48016: {ID: 48016, CellWidth: 4, CellHeight: 3}}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		c.Bag = game.Inventory{}
		for i := 2; i < game.BagSize; i += game.BagColumns {
			c.Bag[i] = game.Item{ID: 101, Count: 1}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := db.Characters(ctx, refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	before := saved[0].Clone()
	now := time.Now()
	if _, _, err = db.DrawLucky(ctx, refs[0], 48016, 1, 1, now, items); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal("fragmented draw accepted", err)
	}
	saved, err = db.Characters(ctx, refs[0].Account)
	if err != nil || saved[0].Bag != before.Bag || saved[0].LuckyDraw != before.LuckyDraw {
		t.Fatal("draw spent allowance without room", err)
	}
	if err = db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error { c.Bag = game.Inventory{}; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, _, err = db.DrawLucky(ctx, refs[0], 48016, 1, 1, now, items); err != nil {
		t.Fatal(err)
	}
	saved, err = db.Characters(ctx, refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	cells, err := saved[0].Bag.Occupancy(items)
	if err != nil || cells[13] != 1 || saved[0].LuckyDraw.Used != 1 {
		t.Fatal("durable footprint/allowance", err)
	}
}
