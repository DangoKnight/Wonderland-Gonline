package world

import (
	"encoding/hex"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestHoneyPatientRemainsDownUntilHandIn(t *testing.T) {
	// Native map 12004 pre-event: honey hand-in mark 13023 equals zero.
	data, err := hex.DecodeString("05df3201000100050000000001000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	var condition [21]byte
	copy(condition[:], data)
	w := eventWorld()
	for _, tc := range []struct {
		name  string
		quest game.Quest
		down  bool
	}{
		{"before collection", game.Quest{}, true},
		{"honey collected", game.Quest{}, true},
		{"handed over", game.Quest{ID: 13023, State: game.InProgress, Step: 1}, false},
		{"quest completed", game.Quest{ID: 13023, State: game.Completed, Step: 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := character()
			if tc.quest.ID != 0 {
				c.Quests[13023] = tc.quest
			}
			if got := w.preEventCondition(c, 12004, condition, NewView()); got != tc.down {
				t.Fatalf("lying pose gate = %v, want %v", got, tc.down)
			}
		})
	}
}
