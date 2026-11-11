package server

import (
	"context"
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

const (
	hillPepperMap                = 12000
	hillPepperActor              = 11
	hillPepperItem               = 30028
	hillPepperProgressMark       = 12042
	hillPepperCompletedMark      = 12043
	kelanQuizPrerequisiteMark    = 12018
	maxHillPepperAcknowledgments = 80
)

func TestStartEventWithoutMatchingBranchDoesNotPanic(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, branch := range []int{-1, 999} {
			s, c, _ := eventFixture(t)
			ev, ok := s.World.Event(c.character.Map, 1)
			if !ok {
				t.Fatal("fixture event missing")
			}
			if active {
				if err := s.startEvent(context.Background(), c, 5, ev, 0, true); err != nil {
					t.Fatal(err)
				}
				if c.event == nil {
					t.Fatal("fixture dialogue did not open")
				}
			}
			if err := s.startEvent(context.Background(), c, 5, ev, branch, true); err != nil {
				t.Fatal(err)
			}
			if c.event != nil {
				t.Fatal("unmatched branch left an interaction open")
			}
		}
	}
}

// This uses only the tracked SQL seed, so a fresh checkout exercises the same
// completion branch as an installed server without relying on local extraction.
func TestHillPepperQuizRewardsExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.db")
	if _, err := assetsql.BuildRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	catalog, err := assetsql.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	s, c, wire := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	c.character.Map = hillPepperMap
	c.character.Quests = map[uint32]game.Quest{
		kelanQuizPrerequisiteMark: {ID: kelanQuizPrerequisiteMark, State: game.InProgress, Step: 1},
	}
	saveBreillatFixture(t, s, c)
	count := func(character *game.Character) int {
		total := 0
		for _, item := range character.Bag {
			if item.ID == hillPepperItem {
				total += int(item.Count)
			}
		}
		return total
	}
	before := count(c.character)
	choices := 0
	play := func() {
		t.Helper()
		handled, err := s.runNPCEvent(context.Background(), c, hillPepperActor)
		if err != nil || !handled || c.event == nil {
			t.Fatalf("quiz did not open: handled=%v err=%v", handled, err)
		}
		for step := 0; c.event != nil && step < maxHillPepperAcknowledgments; step++ {
			wire.Reset()
			if c.event.onChoice != nil {
				choices++
				// Independent captured wire bytes: AC20:9, answer 31, then AC20:6.
				if err := s.eventCommand(context.Background(), c, []byte{20, 9, 31}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if c.event != nil && len(wire.packets(t)) == 0 {
				t.Fatal("quiz stopped responding before completion")
			}
		}
		if c.event != nil {
			t.Fatal("quiz failed to release the player")
		}
	}
	play()
	if choices != 2 {
		t.Fatalf("answered %d questions, expected both quiz questions", choices)
	}
	if got := count(c.character); got != before+1 {
		t.Fatalf("Hill Pepper count=%d, expected %d", got, before+1)
	}
	if mark := c.character.Quests[hillPepperCompletedMark]; mark.Step != 1 {
		t.Fatalf("completion mark not saved: %+v", mark)
	}
	verifyStored := func() {
		t.Helper()
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		for i := range stored {
			if stored[i].ID == c.character.ID {
				if got := count(&stored[i]); got != before+1 {
					t.Fatalf("persisted reward count=%d", got)
				}
				return
			}
		}
		t.Fatal("reward recipient was not persisted")
	}
	verifyStored()
	play()
	if got := count(c.character); got != before+1 {
		t.Fatal("revisiting Emilie duplicated the reward")
	}
	verifyStored()
}
