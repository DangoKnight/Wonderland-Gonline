package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"wonderland-gonline/internal/game"
)

func TestTentFurnitureSearchUsesFloorRotationAndExpandedRows(t *testing.T) {
	items := map[uint16]game.ItemDefinition{1: {ID: 1}, 2: {ID: 2, CellWidth: 2, CellHeight: 1}}
	tent := Tent{Items: []TentItem{{ItemID: 1, X: 42, Y: 42, Floor: 1}}}
	if row, ok := findTentFurnitureSpot(tent, TentItem{ItemID: 2}, items); !ok || row.X != 42 || row.Y != 42 {
		t.Fatal("another floor blocked the preferred anchor", row, ok)
	}
	// Fill the normal floor completely; only expanded rows can accept output.
	for y := 38; y <= 50; y++ {
		for x := 29; x <= 44; x++ {
			tent.Items = append(tent.Items, TentItem{ItemID: 1, X: uint16(x), Y: uint16(y)})
		}
	}
	if _, ok := findTentFurnitureSpot(tent, TentItem{ItemID: 1}, items); ok {
		t.Fatal("full floor reused an occupied anchor")
	}
	tent.Enlarged = true
	if row, ok := findTentFurnitureSpot(tent, TentItem{ItemID: 2, Rotation: 1}, items); !ok || row.X != 29 || row.Y != 32 {
		t.Fatal("enlarged floor search", row, ok)
	}
	tent.Type = 3
	if row, ok := findTentFurnitureSpot(tent, TentItem{ItemID: 1}, items); !ok || row.Y != 27 {
		t.Fatal("grand floor search", row, ok)
	}
	// A rotated two-cell item at (42,42) overlaps (42,43), so the search
	// must move it even though the unrotated footprint would fit.
	tent = Tent{Items: []TentItem{{ItemID: 1, X: 42, Y: 43}}}
	if row, ok := findTentFurnitureSpot(tent, TentItem{ItemID: 2, Rotation: 1}, items); !ok || row.X == 42 && row.Y == 42 {
		t.Fatal("rotation ignored", row, ok)
	}
	if _, ok := findTentFurnitureSpot(tent, TentItem{ItemID: 1, Floor: 1}, items); ok {
		t.Fatal("unavailable second floor accepted")
	}
}

func TestManufacturingFurnitureFullFloorRollsBackWholeBatch(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "compatibility", true: "native"}[native], func(t *testing.T) {
			db, ref, formula, items, _ := manufacturingStoreFixture(t)
			ctx := context.Background()
			now := time.Unix(1000, 0)
			formula.TentOutput = true
			if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
				c.TentReturn = &game.Location{Map: 10019}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			var err error
			if native {
				_, _, _, err = db.StartNativeManufacturing(ctx, ref, 1, formula, 1, 0, now, items)
			} else {
				_, _, _, err = db.StartManufacturing(ctx, ref, 0, formula, now, items)
			}
			if err != nil {
				t.Fatal(err)
			}
			// Leave just one free cell for a two-piece batch. The first insert
			// must roll back when the second piece cannot fit.
			var occupied []TentItem
			for y := 38; y <= 50; y++ {
				for x := 29; x <= 44; x++ {
					if x == 44 && y == 50 {
						continue
					}
					occupied = append(occupied, TentItem{OwnerID: ref.ID, Slot: uint16(len(occupied) + 1), ItemID: 300, Count: 1, X: uint16(x), Y: uint16(y), Metadata: make([]byte, 26)})
				}
			}
			if err = db.transaction(ctx, func(tx *gorm.DB) error { return tx.Create(&occupied).Error }); err != nil {
				t.Fatal(err)
			}
			before, err := db.Characters(ctx, ref.Account)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err = db.CompleteManufacturing(ctx, ref, now.Add(time.Hour), items); !errors.Is(err, game.ErrInventoryFull) {
				t.Fatal("full floor delivery", err)
			}
			home, err := db.Tent(ctx, ref.ID)
			if err != nil || len(home.Items) != len(occupied)+1 {
				t.Fatal("partial furniture delivery survived rollback", len(home.Items), err)
			}
			after, err := db.Characters(ctx, ref.Account)
			if err != nil || after[0].Bag != before[0].Bag {
				t.Fatal("retry spent more ingredients", err)
			}
			if job, err := db.ManufacturingJob(ctx, ref); err != nil || job == nil || job.Count != 2 {
				t.Fatal("recoverable job lost", job, err)
			}
			if _, err = db.db.Exec("DELETE FROM tent_items WHERE owner_id=? AND slot=1", ref.ID); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err = db.CompleteManufacturing(ctx, ref, now.Add(time.Hour), items); err != nil {
				t.Fatal("retry after making space", err)
			}
			home, err = db.Tent(ctx, ref.ID)
			var delivered []TentItem
			for _, row := range home.Items {
				if row.ItemID == 500 {
					delivered = append(delivered, row)
				}
			}
			if err != nil || len(delivered) != 2 || delivered[0].X == delivered[1].X && delivered[0].Y == delivered[1].Y {
				t.Fatal("batch did not use separate free cells", delivered, err)
			}
			if _, _, _, err = db.CompleteManufacturing(ctx, ref, now.Add(time.Hour), items); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("replayed furniture delivery", err)
			}
		})
	}
}
