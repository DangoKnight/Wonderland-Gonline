package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"wonderland-go/internal/game"
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
	if _, err := db.db.Exec("CREATE TRIGGER reject_autosave BEFORE UPDATE ON characters BEGIN SELECT RAISE(ABORT,'injected failure'); END"); err != nil {
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
	if _, err := db.db.Exec("CREATE TRIGGER reject_noop BEFORE UPDATE ON characters BEGIN SELECT RAISE(ABORT,'unexpected write'); END"); err != nil {
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
