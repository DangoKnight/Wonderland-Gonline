package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestAutosavePendingChangesAndStaleSnapshots(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	chars, err := db.Characters(ctx, refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	baseline := chars[0]
	next := baseline.Clone()
	next.X = 123
	written, err := db.AutosaveCharacter(ctx, refs[0], baseline, next)
	if err != nil || !written {
		t.Fatal(written, err)
	}
	written, err = db.AutosaveCharacter(ctx, refs[0], baseline, next)
	if err != nil || written {
		t.Fatal("already durable snapshot rewritten", written, err)
	}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		c.Gold = 99
		c.LuckyDraw = game.LuckyDrawState{Day: "2026-10-02", Used: 3}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// An unchanged cache never undoes an out-of-band grant or consumed allowance.
	if written, err := db.AutosaveCharacter(ctx, refs[0], next, next); err != nil || written {
		t.Fatal(written, err)
	}
	pending := next.Clone()
	pending.Y = 456
	if _, err := db.AutosaveCharacter(ctx, refs[0], next, pending); !errors.Is(err, ErrAutosaveConflict) {
		t.Fatal(err)
	}
	saved, err := db.Characters(ctx, refs[0].Account)
	if err != nil || saved[0].Gold != 99 || saved[0].LuckyDraw.Used != 3 || saved[0].Y == 456 || saved[0].X != 123 {
		t.Fatal(saved, err)
	}
	// Ownership and identity checks also apply to unchanged snapshots.
	if _, err := db.AutosaveCharacter(ctx, refs[1], baseline, next); err == nil {
		t.Fatal("foreign identity accepted")
	}
	foreign := refs[0]
	foreign.Account = refs[1].Account
	if _, err := db.AutosaveCharacter(ctx, foreign, baseline, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	changed := baseline.Clone()
	changed.Name = "Renamed"
	if _, err := db.AutosaveCharacter(ctx, refs[0], baseline, changed); err == nil {
		t.Fatal("identity changed")
	}
	if err := db.DeleteAccount(ctx, refs[0].Account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AutosaveCharacter(ctx, refs[0], baseline, next); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("deleted character recreated", err)
	}
}

func TestAutosaveRollbackAndCancellation(t *testing.T) {
	db, refs := pairFixture(t)
	chars, err := db.Characters(context.Background(), refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	baseline := chars[0]
	next := baseline.Clone()
	next.X = 123
	if _, err := db.db.Exec("CREATE TRIGGER reject_autosave BEFORE UPDATE ON character_state BEGIN SELECT RAISE(ABORT,'injected failure'); END"); err != nil {
		t.Fatal(err)
	}
	if written, err := db.AutosaveCharacter(context.Background(), refs[0], baseline, next); err == nil || written {
		t.Fatal(written, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := db.AutosaveCharacter(ctx, refs[0], baseline, next); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := db.db.Exec("DROP TRIGGER reject_autosave"); err != nil {
		t.Fatal(err)
	}
	if written, err := db.AutosaveCharacter(context.Background(), refs[0], baseline, next); err != nil || !written {
		t.Fatal(written, err)
	}
	next.HP = next.MaxHP + 1
	if _, err := db.AutosaveCharacter(context.Background(), refs[0], baseline, next); err == nil {
		t.Fatal("invalid snapshot accepted")
	}
}

func TestAutosaveUnchangedDoesNotWriteAndNormalizesClone(t *testing.T) {
	db, refs := pairFixture(t)
	chars, err := db.Characters(context.Background(), refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	baseline := chars[0].Clone()
	next := baseline.Clone()
	if _, err := db.db.Exec("CREATE TRIGGER reject_noop BEFORE UPDATE ON character_state BEGIN SELECT RAISE(ABORT,'unexpected write'); END"); err != nil {
		t.Fatal(err)
	}
	if written, err := db.AutosaveCharacter(context.Background(), refs[0], baseline, next); err != nil || written {
		t.Fatal(written, err)
	}
	if _, err := db.db.Exec("DROP TRIGGER reject_noop"); err != nil {
		t.Fatal(err)
	}
	next.X = 123
	if written, err := db.AutosaveCharacter(context.Background(), refs[0], baseline, next); err != nil || !written {
		t.Fatal("cloned nil collections caused a conflict", written, err)
	}
}

func TestPositionCheckpointDoesNotRewriteOwnedRows(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	chars, err := db.Characters(ctx, refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	baseline := chars[0].Clone()
	if err = db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error { c.Gold = 77; return nil }); err != nil {
		t.Fatal(err)
	}
	next := baseline.Clone()
	next.Gold = 77
	next.X = 123
	if _, err = db.db.Exec("CREATE TRIGGER reject_child_rewrite BEFORE DELETE ON character_items BEGIN SELECT RAISE(ABORT,'position rewrote inventory'); END"); err != nil {
		t.Fatal(err)
	}
	if written, err := db.AutosaveCharacter(ctx, refs[0], baseline, next); err != nil || !written {
		t.Fatal(written, err)
	}
	saved, err := db.Characters(ctx, refs[0].Account)
	if err != nil || saved[0].Gold != 77 || saved[0].X != 123 || saved[0].Bag != baseline.Bag {
		t.Fatal(saved, err)
	}
	// Even a same-map out-of-band position change must defeat a stale checkpoint.
	if _, err = db.db.Exec("UPDATE character_state SET x=999 WHERE character_id=?", refs[0].ID); err != nil {
		t.Fatal(err)
	}
	pending := next.Clone()
	pending.Y = 456
	if _, err := db.AutosaveCharacter(ctx, refs[0], next, pending); !errors.Is(err, ErrAutosaveConflict) {
		t.Fatal("external warp overwritten", err)
	}
}

// TestAutosaveRestingVitals: buffered HP/SP save through the guarded state
// update and never overwrite a vital SQL changed since the checkpoint.
func TestAutosaveRestingVitals(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	chars, err := db.Characters(ctx, refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	baseline := chars[0]
	next := baseline.Clone()
	next.X, next.HP = 321, 0
	if written, err := db.AutosaveCharacter(ctx, refs[0], baseline, next); err != nil || !written {
		t.Fatal(written, err)
	}
	saved, err := db.Characters(ctx, refs[0].Account)
	if err != nil || saved[0].HP != 0 || saved[0].X != 321 {
		t.Fatal(saved[0].HP, err)
	}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		c.HP = 1
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	pending := next.Clone()
	pending.X = 322
	if _, err := db.AutosaveCharacter(ctx, refs[0], next, pending); !errors.Is(err, ErrAutosaveConflict) {
		t.Fatal(err)
	}
}

func TestAutosavePetRestingVitals(t *testing.T) {
	db, refs := pairFixture(t)
	ctx, ref := context.Background(), refs[0]
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error {
		c.Pets = []game.Pet{{ID: 123, Slot: 1, Level: 1, HP: 1, MaxHP: 20, MaxSP: 10}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chars, err := db.Characters(ctx, ref.Account)
	if err != nil {
		t.Fatal(err)
	}
	baseline := chars[0]
	next := baseline.Clone()
	next.X, next.Pets[0].HP, next.Pets[0].SP = 321, 5, 3
	for _, table := range []string{"character_items", "character_pets", "character_pet_skills"} {
		if _, err := db.db.Exec("CREATE TRIGGER reject_rest_delete_" + table + " BEFORE DELETE ON " + table + " BEGIN SELECT RAISE(ABORT,'rest rewrote owned rows'); END"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.db.Exec("CREATE TRIGGER reject_pet_rest BEFORE UPDATE ON character_pets BEGIN SELECT RAISE(ABORT,'pet checkpoint failed'); END"); err != nil {
		t.Fatal(err)
	}
	if written, err := db.AutosaveCharacter(ctx, ref, baseline, next); err == nil || written {
		t.Fatal("pet failure committed a partial checkpoint", written, err)
	}
	saved, err := db.Characters(ctx, ref.Account)
	if err != nil || saved[0].X != baseline.X || saved[0].Pets[0].HP != 1 {
		t.Fatal("failed pet update did not roll back position", err)
	}
	if _, err := db.db.Exec("DROP TRIGGER reject_pet_rest"); err != nil {
		t.Fatal(err)
	}
	if written, err := db.AutosaveCharacter(ctx, ref, baseline, next); err != nil || !written {
		t.Fatal("retry", written, err)
	}
	saved, err = db.Characters(ctx, ref.Account)
	if err != nil || saved[0].X != 321 || saved[0].Pets[0].HP != 5 || saved[0].Pets[0].SP != 3 {
		t.Fatal("pet recovery not checkpointed", saved, err)
	}
	if written, err := db.AutosaveCharacter(ctx, ref, next, next); err != nil || written {
		t.Fatal("clean pet checkpoint rewritten", written, err)
	}
	// A durable vital change must defeat a stale recovery snapshot, without
	// undoing the change or saving the snapshot's newer walking position.
	if _, err := db.db.Exec("UPDATE character_pets SET hp=2 WHERE character_id=? AND location='party'", ref.ID); err != nil {
		t.Fatal(err)
	}
	pending := next.Clone()
	pending.X, pending.Pets[0].HP = 322, 7
	if _, err := db.AutosaveCharacter(ctx, ref, next, pending); !errors.Is(err, ErrAutosaveConflict) {
		t.Fatal("stale pet vitals accepted", err)
	}
	saved, err = db.Characters(ctx, ref.Account)
	if err != nil || saved[0].X != 321 || saved[0].Pets[0].HP != 2 {
		t.Fatal("stale recovery overwrote SQL", err)
	}
}
