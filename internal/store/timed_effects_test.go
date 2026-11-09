package store

import (
	"context"
	"errors"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestTimedEffectsPersistAndCheckpointConflict(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	ref := refs[0]
	initial := game.TimedEffects{EXPKind: game.TimedItemPotion, EXPTenths: 100, EXPSeconds: 7200, TrainingSeconds: 3600}
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error { c.TimedEffects = initial; return nil }); err != nil {
		t.Fatal(err)
	}
	chars, err := db.Characters(ctx, ref.Account)
	if err != nil {
		t.Fatal(err)
	}
	baseline := chars[0]
	if baseline.TimedEffects != initial {
		t.Fatal("roundtrip", baseline.TimedEffects)
	}
	next := baseline.Clone()
	next.TimedEffects.Advance(10, true)
	if written, err := db.AutosaveCharacter(ctx, ref, baseline, next); err != nil || !written {
		t.Fatal(written, err)
	}
	chars, err = db.Characters(ctx, ref.Account)
	if err != nil || chars[0].TimedEffects != next.TimedEffects {
		t.Fatal("checkpoint", err, chars)
	}
	if err := db.UpdateCharacter(ctx, ref.Account, ref.ID, func(c *game.Character) error { c.TimedEffects.TrainingSeconds += 3600; return nil }); err != nil {
		t.Fatal(err)
	}
	pending := next.Clone()
	pending.TimedEffects.Advance(10, true)
	if _, err := db.AutosaveCharacter(ctx, ref, next, pending); !errors.Is(err, ErrAutosaveConflict) {
		t.Fatal("stale timer overwrote activation", err)
	}
}

func TestTimedEffectsUpgradePreservesExistingCharacters(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	before, err := db.Characters(ctx, refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the immediately preceding schema without timer columns.
	for _, column := range []string{"timed_exp_kind", "timed_exp_tenths", "timed_exp_seconds", "timed_exp_paused", "timed_training_seconds"} {
		if _, err = db.db.Exec("ALTER TABLE character_state DROP COLUMN " + column); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.db.Exec("PRAGMA user_version=24"); err != nil {
		t.Fatal(err)
	}
	if err = db.migrate(); err != nil {
		t.Fatal(err)
	}
	after, err := db.Characters(ctx, refs[0].Account)
	if err != nil {
		t.Fatal(err)
	}
	if !CharacterSnapshotsEqual(before[0], after[0]) {
		t.Fatal("timer migration changed existing character")
	}
}
