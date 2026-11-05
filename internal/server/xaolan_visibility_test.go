package server

import (
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestInstalledXaolanLeavesHouseOnNextVisit(t *testing.T) {
	const (
		houseMap              = 12002
		xaolanActor           = 2
		grandmotherActor      = 3
		rescueMark            = 13032
		rescuedMark           = 13033
		laterRequestMark      = 13004
		laterGrandmotherActor = 6
		laterXaolanActor      = 7
	)
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	c := &game.Character{Map: houseMap, Quests: map[uint32]game.Quest{}}
	v := world.NewView()
	w.Sync(c, v, true)
	for _, actor := range []uint16{xaolanActor, grandmotherActor} {
		if v.Hidden[actor] || !w.VisibleIn(c, v, houseMap, actor) {
			t.Fatal("missing before rescue", actor)
		}
	}
	c.Quests[rescueMark] = game.Quest{ID: rescueMark, State: game.Completed}
	c.Quests[rescuedMark] = game.Quest{ID: rescuedMark, State: game.InProgress, Step: 1}
	w.Sync(c, v, false)
	for _, actor := range []uint16{xaolanActor, grandmotherActor} {
		if v.Hidden[actor] || !w.VisibleIn(c, v, houseMap, actor) {
			t.Fatal("disappeared during rescue visit", actor)
		}
	}
	// Map exit/entry resets visit-local state; durable quest marks remain intact.
	for range 2 {
		v.Reset()
		w.Sync(c, v, true)
		for _, actor := range []uint16{xaolanActor, grandmotherActor} {
			if !v.Hidden[actor] || w.VisibleIn(c, v, houseMap, actor) {
				t.Fatal("returned after rescue", actor)
			}
		}
	}
	// Native later quest stages may legitimately introduce the alternate actors.
	c.Quests[laterRequestMark] = game.Quest{ID: laterRequestMark, State: game.InProgress, Step: 1}
	v.Reset()
	w.Sync(c, v, true)
	for _, actor := range []uint16{laterGrandmotherActor, laterXaolanActor} {
		if v.Hidden[actor] || !w.VisibleIn(c, v, houseMap, actor) {
			t.Fatal("later quest actor suppressed", actor)
		}
	}
}
