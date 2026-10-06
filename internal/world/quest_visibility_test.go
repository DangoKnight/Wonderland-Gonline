package world

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestQuestVisibilityRegisteredListPrecedence(t *testing.T) {
	w := fixture()
	w.catalog.QuestVisibility = []assets.QuestVisibility{{ID: 99, Map: 12345, QuestActors: assets.QuestActors{Spawn: []uint16{1}, Despawn: []uint16{4}}, Steps: []assets.QuestVisibilityStep{{Index: 2, QuestActors: assets.QuestActors{Spawn: []uint16{3}, Despawn: []uint16{4}}}}}}
	for _, tc := range []struct {
		name string
		q    game.Quest
		want [3]bool
	}{
		{"not started", game.Quest{}, [3]bool{false, false, true}},
		{"different step", game.Quest{State: game.InProgress, Step: 1}, [3]bool{false, false, true}},
		{"active step", game.Quest{State: game.InProgress, Step: 2}, [3]bool{false, true, false}},
		{"completed", game.Quest{State: game.Completed, Step: 2}, [3]bool{true, false, false}},
		{"failed", game.Quest{State: game.Failed, Step: 2}, [3]bool{false, false, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := character(tc.q)
			c.Quests[99] = tc.q
			for i, id := range []uint16{1, 3, 4} {
				if got := w.Visible(c, 12345, id); got != tc.want[i] {
					t.Fatalf("actor %d=%v, want %v", id, got, tc.want[i])
				}
			}
		})
	}
	// Recruited companions and explicit administrator visibility retain precedence.
	c := character(game.Quest{ID: 99, State: game.Completed})
	v := NewView()
	v.AdminActors = map[uint16]bool{1: false}
	if w.VisibleIn(c, v, 12345, 1) {
		t.Fatal("quest list overrode admin visibility")
	}
	if got, owned := w.questVisibility(c, 20000, 1); got || owned {
		t.Fatal("list leaked to another map")
	}
	// Earlier registered spawn list makes the decision, as in the reference.
	w.catalog.QuestVisibility = append(w.catalog.QuestVisibility, assets.QuestVisibility{ID: 100, Map: 12345, QuestActors: assets.QuestActors{Despawn: []uint16{1}}})
	c.Quests[100] = game.Quest{State: game.Completed}
	if !w.Visible(c, 12345, 1) {
		t.Fatal("registration order lost")
	}
}

func TestQuestVisibilitySyncHidesAndRestoresActors(t *testing.T) {
	w := fixture()
	w.catalog.QuestVisibility = []assets.QuestVisibility{{ID: 99, Map: 12345, QuestActors: assets.QuestActors{Spawn: []uint16{1}}}}
	c := character()
	v := NewView()
	w.Sync(c, v, false)
	if !v.Hidden[1] {
		t.Fatal("spawn-only actor initially exposed")
	}
	c.Quests[99] = game.Quest{State: game.Completed}
	packets := w.Sync(c, v, false)
	shown := false
	for _, p := range packets {
		if len(p) >= 4 && bytes.Equal(p[:4], []byte{22, 4, 1, 0}) {
			shown = true
		}
	}
	if !shown || v.Hidden[1] {
		t.Fatal("completed quest did not restore actor", packets)
	}
}
