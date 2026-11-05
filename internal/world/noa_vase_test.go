package world

import (
	"bytes"
	"encoding/hex"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// The first four branch headers of Kelan event 56, copied from EVE.
func TestNoaQuizEntryDistinguishesMissingAndFailedScore(t *testing.T) {
	const kelanMap = 12000
	const noaEvent = 56
	const quizScore = 50630
	const recruitment = 50631
	var branches []assets.Branch
	for i, raw := range []string{
		"05c6c5010001000605000000020000000000000000",
		"05c7c5010001000500000000000000000000000000",
		"05c6c5020001000000000000020000000000000000",
		"05c7c5010001000500000000000000000000000000",
	} {
		b := assets.Branch{Index: byte(i + 1)}
		data, err := hex.DecodeString(raw)
		if err != nil {
			t.Fatal(err)
		}
		copy(b.Condition[:], data)
		if i%2 == 0 {
			b.Operations = []assets.Operation{{}}
		}
		branches = append(branches, b)
	}
	ev := assets.Event{ClickID: noaEvent, Branches: branches}
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{kelanMap: {ID: kelanMap, Events: []assets.Event{ev}}}})
	for _, tc := range []struct {
		name   string
		quests map[uint32]game.Quest
		want   int
	}{
		{"first meeting", map[uint32]game.Quest{}, 2},
		{"failed attempt", map[uint32]game.Quest{quizScore: {State: game.InProgress, Step: 1}}, 0},
		{"zero score attempt", map[uint32]game.Quest{quizScore: {State: game.InProgress, Step: 0}}, 0},
		{"retry cleared old score", map[uint32]game.Quest{quizScore: {State: game.Completed, Step: 1}}, 2},
		{"perfect score", map[uint32]game.Quest{quizScore: {State: game.InProgress, Step: 5}}, -1},
		{"already recruited", map[uint32]game.Quest{recruitment: {State: game.InProgress, Step: 1}}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &game.Character{Map: kelanMap, Quests: tc.quests}
			if got := w.FindBranch(c, nil, kelanMap, &ev, 0, 0, 0, -1); got != tc.want {
				t.Fatalf("branch=%d want %d", got, tc.want)
			}
		})
	}
}

func TestBreakableContainerRetainsClaimedFrameOnMapEntry(t *testing.T) {
	const houseMap = 12002
	const vaseActor = 1
	const vaseTemplate = 16006
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{houseMap: {ID: houseMap, NPCs: []assets.MapNPC{{ClickID: vaseActor, Template: vaseTemplate, Flags: 5, X: 507, Y: 286}}}}})
	c := &game.Character{Map: houseMap, Quests: map[uint32]game.Quest{}}
	frame := func() []byte {
		for _, p := range w.MapInfo(c, NewView(), nil) {
			if len(p) > 1 && p[0] == 22 && p[1] == 4 {
				return p
			}
		}
		return nil
	}
	// Independent AC22:4 fixtures for intact and broken vase on re-entry.
	intact := []byte{22, 4, 1, 0, 255, 0, 251, 1, 30, 1, 1, 0, 0, 0, 0, 0}
	broken := []byte{22, 4, 1, 0, 1, 0, 251, 1, 30, 1, 1, 0, 0, 0, 0, 0}
	if got := frame(); !bytes.Equal(got, intact) {
		t.Fatalf("intact: %x", got)
	}
	c.Quests[ChestKey(houseMap, vaseActor)] = game.Quest{State: game.Completed, Step: 1}
	if got := frame(); !bytes.Equal(got, broken) {
		t.Fatalf("broken: %x", got)
	}
}
