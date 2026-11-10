package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

// A missing completion branch used to index Branches[-1] and take the whole
// server down, which drops every connected session — the captured native client
// lost its connection at exactly this point.
func TestStartEventWithoutMatchingBranchDoesNotPanic(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Skipf("installed assets unavailable: %v", err)
	}
	s, c, _ := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	c.character.Map = 12000
	c.character.Quests = map[uint32]game.Quest{}
	saveBreillatFixture(t, s, c)

	ev, ok := s.World.Event(12000, 1)
	if !ok {
		var id uint16
		for _, e := range s.Assets.Maps[12000].NPCs {
			if len(e.Events) > 0 {
				id = uint16(e.Events[0])
				break
			}
		}
		if id == 0 {
			t.Skip("no event on the quiz map")
		}
		if ev, ok = s.World.Event(12000, id); !ok {
			t.Skipf("event %d is not loaded", id)
		}
	}
	if err := s.startEvent(context.Background(), c, 11, ev, -1, true); err != nil {
		t.Fatalf("start without a branch: %v", err)
	}
	if c.event != nil {
		t.Fatal("opened an interaction without a matching branch")
	}
}

// The captured session 2026-11-10 19:59 ends after the Hill Pepper quiz answers
// its success line: the client acknowledges, the server sends nothing more, and
// both the native and the ported client lose the session. Drive the installed
// quest through the same packets and require the event to close.
func TestInstalledHillPepperQuizCloses(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Skipf("installed assets unavailable: %v", err)
	}
	s, c, wire := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)

	const quizMap = 12000
	const quizClick = 11 // 0x0b, the actor the captured frames carry.
	if _, ok := s.Assets.Maps[quizMap]; !ok {
		t.Skipf("map %d is not in the installed catalog", quizMap)
	}
	c.character.Map = quizMap
	c.character.Quests = map[uint32]game.Quest{}
	saveBreillatFixture(t, s, c)

	var npc *assets.MapNPC
	for i := range s.Assets.Maps[quizMap].NPCs {
		if s.Assets.Maps[quizMap].NPCs[i].ClickID == quizClick {
			npc = &s.Assets.Maps[quizMap].NPCs[i]
			break
		}
	}
	if npc == nil || len(npc.Events) == 0 {
		t.Skipf("quiz actor %d has no events", quizClick)
	}
	ev, ok := s.World.Event(quizMap, uint16(npc.Events[0]))
	if !ok {
		t.Skipf("event %d is not loaded", npc.Events[0])
	}
	branch := s.World.FindBranch(c.character, c.view, quizMap, ev, world.TriggerEntry, 0, 0, -1)
	if branch < 0 {
		// The quiz enters on its first branch; FindBranch only resolves the
		// completion branches once the marks exist.
		branch = 0
	}
	if err := s.startEvent(context.Background(), c, quizClick, ev, branch, true); err != nil {
		t.Fatalf("start: %v", err)
	}
	if c.event == nil {
		t.Fatalf("the quiz did not open (branch %d of %d)", branch, len(ev.Branches))
	}

	// The native client answers option 1 with 30+option and acks every page.
	steps := 0
	for ; c.event != nil && steps < 200; steps++ {
		wire.Reset()
		var err error
		if c.event.onChoice != nil {
			if err = s.eventCommand(context.Background(), c, []byte{20, 9, 31}); err == nil {
				err = s.eventCommand(context.Background(), c, []byte{20, 6})
			}
		} else {
			err = s.eventCommand(context.Background(), c, []byte{20, 6})
		}
		if err != nil {
			t.Fatalf("step %d: %v", steps, err)
		}
		if c.event != nil && len(wire.packets(t)) == 0 {
			t.Fatalf("step %d answered with no packet: the client waits forever", steps)
		}
	}
	if c.event != nil {
		t.Fatalf("event still open after %d acknowledgements", steps)
	}
}
