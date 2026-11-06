package store

import (
	"context"
	"fmt"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestCharacterGroupRollsBackRecipients(t *testing.T) {
	db, refs := pairFixture(t)
	if _, err := db.db.Exec(fmt.Sprintf("CREATE TRIGGER reject_group BEFORE UPDATE ON character_state WHEN NEW.character_id=%d BEGIN SELECT RAISE(ABORT,'recipient failure'); END", refs[1].ID)); err != nil {
		t.Fatal(err)
	}
	err := db.UpdateCharacters(context.Background(), refs[:], func(chars []game.Character) error { chars[0].Gold += 10; chars[1].Gold += 10; return nil })
	if err == nil || pairGold(t, db, refs) != [2]uint32{100, 100} {
		t.Fatal("partial recipient commit", err)
	}
}
