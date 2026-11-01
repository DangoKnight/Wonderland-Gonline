package world

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestQuestLifecycleAndCharacterSubjects(t *testing.T) {
	w, c := eventWorld(), character()
	check := func(d [21]byte, expected bool) {
		t.Helper()
		if got := w.condition(c, nil, c.Map, nil, d); got != expected {
			t.Fatalf("condition %v: %v want %v", d, got, expected)
		}
	}
	check(cond(5, 700, 1, 0, 3, 9), false) // Absent is not an accepted early step.
	check(cond(5, 700, 1, 0, 5, 0), true)
	check(cond(5, 700, 2, 0, 5, 2), true)
	c.Quests[700] = game.Quest{ID: 700, State: game.InProgress, Step: 3}
	check(cond(5, 700, 1, 0, 3, 9), true)
	check(cond(5, 700, 2, 0, 5, 1), true)
	c.Quests[700] = game.Quest{ID: 700, State: game.Completed}
	check(cond(5, 700, 2, 0, 5, 3), true)
	check(cond(5, 700, 1, 0, 5, 1), true)
	c.Body, c.Element, c.Job = 3, 4, 2
	check(cond(2, 3, 7, 0, 0, 0), true)
	check(cond(2, 3, 8, 0, 0, 0), false)
	check(cond(2, 4, 4, 0, 0, 0), true)
	check(cond(2, 5, 2, 0, 0, 0), true)
	check(cond(2, 5, 2, 0, 6, 0), false)
	check(cond(13, 1, 0, 0, 5, 1), true)
}
