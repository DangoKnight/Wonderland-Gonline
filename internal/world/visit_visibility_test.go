package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestVisitVisibilityRetainsSceneUntilReset(t *testing.T) {
	const testMap = 10017
	const testActor = 4
	const progressMark = 13032
	catalog := &assets.Catalog{
		Maps: map[uint16]assets.Map{testMap: {ID: testMap, NPCs: []assets.MapNPC{{ClickID: testActor, Flags: 1}}}},
		Story: assets.StoryDefinitions{Rules: []assets.StoryRule{
			{ID: "later_hidden", Kind: assets.StoryFinalVisibility, Scope: assets.StoryScope{Maps: []uint16{testMap}, Actors: []uint16{testActor}}, Gates: []assets.StoryGate{{Source: "mark_state", ID: progressMark, Op: "eq", Value: storyMarkStateCompleted}}},
			{ID: "keep_visit", Kind: assets.StoryVisitVisibility, Scope: assets.StoryScope{Maps: []uint16{testMap}, Actors: []uint16{testActor}}},
		}},
	}
	w := New(catalog)
	c := &game.Character{Map: testMap, Quests: map[uint32]game.Quest{}}
	v := NewView()
	w.Sync(c, v, true)
	c.Quests[progressMark] = game.Quest{ID: progressMark, State: game.Completed}
	w.Sync(c, v, false)
	if v.Hidden[testActor] {
		t.Fatal("quest update removed actor mid-visit")
	}
	// Explicit scene directions take precedence over the retained visibility.
	v.Actors[testActor] = false
	w.Sync(c, v, false)
	if !v.Hidden[testActor] {
		t.Fatal("explicit hide was ignored")
	}
	v.Reset()
	w.Sync(c, v, true)
	if !v.Hidden[testActor] {
		t.Fatal("new visit did not apply durable progress")
	}
}
