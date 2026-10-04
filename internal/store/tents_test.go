package store

import (
	"context"
	"errors"
	"testing"
	"wonderland-go/internal/game"
)

func TestTentFurnitureAtomicOwnershipMetadataAndOrdinalIndex(t *testing.T) {
	s, refs, items := economyStoreFixture(t)
	ctx := context.Background()
	home, err := s.EnsureTent(ctx, refs[0], 39062, 39064, []TentItem{{ItemID: 32176, X: 43, Y: 42}})
	if err != nil || len(home.Items) != 1 {
		t.Fatal(home, err)
	}
	chars, _ := s.Characters(ctx, refs[0].Account)
	expected := chars[0].Bag[0]
	if _, err = s.PlaceTentItem(ctx, refs[1], 1, 50, 60, expected); err == nil {
		t.Fatal("another owner wrote furniture")
	}
	next, err := s.PlaceTentItem(ctx, refs[0], 1, 50, 60, expected)
	if err != nil || next.Bag[0].Count != 9 {
		t.Fatal(next, err)
	}
	if _, err = s.PlaceTentItem(ctx, refs[0], 1, 50, 60, expected); !errors.Is(err, game.ErrTradeChanged) {
		t.Fatal("stale placement accepted", err)
	}
	_, _, err = s.PickUpTentItem(ctx, refs[0], 0, items)
	if err != nil {
		t.Fatal(err)
	}
	// The remaining furniture is stored at PK slot 1, but its wire index is 0.
	if err = s.MoveTentItem(ctx, refs[0], 0, 99, 88, 0, 2); err != nil {
		t.Fatal(err)
	}
	home, err = s.Tent(ctx, refs[0].ID)
	if err != nil || len(home.Items) != 1 || home.Items[0].Slot != 1 || home.Items[0].X != 99 || home.Items[0].Rotation != 2 || home.Items[0].Damage != 3 || home.Items[0].Metadata[4] != 99 {
		t.Fatal(home, err)
	}
	if err = s.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 32176, Count: 50, Damage: 8}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.PickUpTentItem(ctx, refs[0], 0, items); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	home, _ = s.Tent(ctx, refs[0].ID)
	if len(home.Items) != 1 {
		t.Fatal("failed pickup removed furniture")
	}
	if err = s.SetTentLocked(ctx, refs[0], true); err != nil {
		t.Fatal(err)
	}
	home, err = s.EnsureTent(ctx, refs[0], 1, 2, nil)
	if err != nil || !home.Locked || home.Floor != 39062 || len(home.Items) != 1 {
		t.Fatal("initialization overwrote home", home, err)
	}
}

func TestTentRecoveryAndReservationsUseTypedState(t *testing.T) {
	s, refs, items := economyStoreFixture(t)
	ctx := context.Background()
	if err := s.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		c.Map = 63507
		c.TentReturn = &game.Location{Map: 12000, X: 10, Y: 20}
		c.Bag[0].Locked = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chars, err := s.Characters(ctx, refs[0].Account)
	if err != nil || chars[0].TentReturn == nil || chars[0].TentReturn.Y != 20 || chars[0].Bag[0].Locked {
		t.Fatal("wrong durable projection", chars, err)
	}
	snapshot := chars[0].Clone()
	snapshot.Bag[0].Locked = true
	if _, err = s.SendParcel(ctx, refs[0], refs[1].ID, "S", "B", 100, 1, 1, items, snapshot); !errors.Is(err, game.ErrItemLocked) {
		t.Fatal("parcel consumed reservation", err)
	}
	after, _ := s.Characters(ctx, refs[0].Account)
	if after[0].Gold != chars[0].Gold || after[0].Bag != chars[0].Bag {
		t.Fatal("failed parcel partially saved")
	}
	if written, err := s.AutosaveCharacter(ctx, refs[0], chars[0], snapshot); err != nil || written {
		t.Fatal("transient reservation triggered autosave", err)
	}
	rows, _ := s.Parcels(ctx, refs[1].ID)
	if len(rows) != 0 {
		t.Fatal("failed parcel created escrow")
	}
	clone := chars[0].Clone()
	clone.TentReturn.X++
	if chars[0].TentReturn.X != 10 {
		t.Fatal("cloned return aliases original")
	}
}

func TestTentVersionNineMigrationPreservesExistingCharacter(t *testing.T) {
	s, refs, _ := economyStoreFixture(t)
	before, err := s.Characters(context.Background(), refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	var path string
	if err = s.db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{"DROP TABLE tent_items", "DROP TABLE tents", "ALTER TABLE character_state DROP COLUMN tent_return_present", "ALTER TABLE character_state DROP COLUMN tent_return_map", "ALTER TABLE character_state DROP COLUMN tent_return_x", "ALTER TABLE character_state DROP COLUMN tent_return_y", "PRAGMA user_version=9"} {
		if _, err = s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	after, err := upgraded.Characters(context.Background(), refs[0].Account)
	if err != nil || CharacterVersion(before[0]) != CharacterVersion(after[0]) {
		t.Fatal("v9 upgrade changed character", err)
	}
	if _, err = upgraded.EnsureTent(context.Background(), refs[0], 39062, 39064, nil); err != nil {
		t.Fatal(err)
	}
}
