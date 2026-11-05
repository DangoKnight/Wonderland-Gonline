package server

import (
	"context"
	"testing"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestInstalledLostDogVisibleBeforeAcceptance(t *testing.T) {
	const (
		kelanMap         = 12000
		strayDogActor    = 20
		returnedDogActor = 28
		strayDogEvent    = 26
		requestMark      = 13046
		completedMark    = 13047
		unacceptedBranch = 1
		recoveryBranch   = 3
		acceptedStep     = 1
		recoveredStep    = 2
	)
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	event, ok := w.Event(kelanMap, strayDogEvent)
	if !ok {
		t.Fatal("missing native dog event")
	}
	for _, stage := range []struct {
		name            string
		quests          map[uint32]game.Quest
		stray, returned bool
		branch          byte
	}{
		{"fresh", map[uint32]game.Quest{}, true, false, unacceptedBranch},
		{"accepted", map[uint32]game.Quest{requestMark: {ID: requestMark, State: game.InProgress, Step: acceptedStep}}, true, false, recoveryBranch},
		{"recovered", map[uint32]game.Quest{requestMark: {ID: requestMark, State: game.InProgress, Step: recoveredStep}}, false, true, 0},
		{"finished", map[uint32]game.Quest{requestMark: {ID: requestMark, State: game.Completed, Step: recoveredStep}, completedMark: {ID: completedMark, State: game.InProgress, Step: acceptedStep}}, false, true, 0},
	} {
		t.Run(stage.name, func(t *testing.T) {
			c := &game.Character{Map: kelanMap, Quests: stage.quests}
			if w.VisibleIn(c, world.NewView(), kelanMap, strayDogActor) != stage.stray || w.VisibleIn(c, world.NewView(), kelanMap, returnedDogActor) != stage.returned {
				t.Fatal("dog visibility does not match native quest stage")
			}
			index := w.FindBranch(c, world.NewView(), kelanMap, event, 0, 0, 0, -1)
			branch := byte(0)
			if index >= 0 {
				branch = event.Branches[index].Index
			}
			if branch != stage.branch {
				t.Fatal("wrong dog interaction branch", branch)
			}
		})
	}
	// Clicking before acceptance must not recover the dog or advance the quest.
	s, players, _ := worldFixture(t)
	c := players[0]
	s.Assets, s.World = catalog, w
	c.character.Map, c.character.X, c.character.Y = kelanMap, 2535, 1543
	c.character.Quests = map[uint32]game.Quest{}
	saveBreillatFixture(t, s, c)
	branch := w.FindBranch(c.character, c.view, kelanMap, event, 0, 0, 0, -1)
	if err := s.startEvent(context.Background(), c, strayDogActor, event, branch, true); err != nil {
		t.Fatal(err)
	}
	finishBreillatDialogue(t, s, c)
	if _, exists := c.character.Quests[requestMark]; exists || !w.Visible(c.character, kelanMap, strayDogActor) {
		t.Fatal("unaccepted interaction recovered the dog")
	}
	// The same interaction is allowed to recover it after accepting the request.
	c.character.Quests[requestMark] = game.Quest{ID: requestMark, State: game.InProgress, Step: acceptedStep}
	saveBreillatFixture(t, s, c)
	branch = w.FindBranch(c.character, c.view, kelanMap, event, 0, 0, 0, -1)
	if err := s.startEvent(context.Background(), c, strayDogActor, event, branch, true); err != nil {
		t.Fatal(err)
	}
	finishBreillatDialogue(t, s, c)
	if c.event != nil || c.character.Quests[requestMark].Step != recoveredStep {
		t.Fatal("accepted interaction did not recover the dog", c.character.Quests)
	}
	if w.VisibleIn(c.character, world.NewView(), kelanMap, strayDogActor) || !w.VisibleIn(c.character, world.NewView(), kelanMap, returnedDogActor) {
		t.Fatal("recovered dog did not return to its owner")
	}
}
