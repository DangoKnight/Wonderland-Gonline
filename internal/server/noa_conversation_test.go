package server

import (
	"bytes"
	"context"
	"testing"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

const (
	noaRecruitmentMap    = 12000
	noaRecruitmentEvent  = 56
	noaRecruitmentActor  = 41
	noaCompanionTemplate = 25020
	noaQuizScoreMark     = 50630
)

// Exercise the imported EVE program, including the mode-zero answer ACKs sent
// by both clients. Conversation links must never publish an unlock/resume.
// aLogin acknowledges steps but does not choose the next EVE branch; the scoped
// fallback and native evidence are documented in WORLD_COMBAT_PARITY.md.
func TestInstalledNoaRecruitmentConversation(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name             string
		retry, returning bool
	}{
		{name: "perfect"}, {name: "failed_then_retry", retry: true}, {name: "return_after_failed_attempt", returning: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, wire := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			c.character.Map = noaRecruitmentMap
			c.character.Pets = nil
			c.character.Quests = map[uint32]game.Quest{}
			if tc.returning {
				c.character.Quests[noaQuizScoreMark] = game.Quest{ID: noaQuizScoreMark, State: game.InProgress, Step: 1}
			}
			saveBreillatFixture(t, s, c)
			ev, ok := s.World.Event(noaRecruitmentMap, noaRecruitmentEvent)
			if !ok {
				t.Fatal("native Noa event missing")
			}
			branch := s.World.FindBranch(c.character, c.view, noaRecruitmentMap, ev, world.TriggerEntry, 0, 0, -1)
			if err := s.startEvent(context.Background(), c, noaRecruitmentActor, ev, branch, true); err != nil {
				t.Fatal(err)
			}
			// Question 16 accepts the quiz; 18..22 are its five answers.
			answers := []byte{30, 31, 31, 30, 31, 30}
			if tc.retry {
				answers = append([]byte{30, 30, 30, 31, 30, 30}, answers...)
			}
			answer := 0
			seen := map[byte]bool{}
			for step := 0; c.event != nil && step < 120; step++ {
				es := c.event
				seen[es.ev.Branches[es.branch].Index] = true
				wire.Reset()
				if es.onChoice != nil {
					if answer == len(answers) {
						t.Fatal("unexpected extra question")
					}
					err = s.eventCommand(context.Background(), c, []byte{20, 9, answers[answer]})
					answer++
					if err == nil {
						err = s.eventCommand(context.Background(), c, []byte{20, 6})
					}
				} else {
					err = s.eventCommand(context.Background(), c, []byte{20, 6})
				}
				if err != nil {
					t.Fatal(err)
				}
				if c.event != nil {
					for _, p := range wire.packets(t) {
						if bytes.Equal(p, []byte{20, 8}) || bytes.Equal(p, []byte{6, 2, 0}) {
							t.Fatal("conversation released before recruitment", p, seen)
						}
					}
				}
			}
			if c.event != nil || answer != len(answers) || len(c.character.Pets) != 1 || c.character.Pets[0].ID != noaCompanionTemplate {
				t.Fatalf("incomplete recruitment: answers=%d event=%v pets=%v branches=%v", answer, c.event, c.character.Pets, seen)
			}
			if !seen[21] || tc.retry && !seen[17] || tc.returning && !seen[1] {
				t.Fatal("missing result/retry stage", seen)
			}
		})
	}
}

func TestInstalledNoaConversationCanDeclineOrCancel(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, answer := range []byte{31, 40} {
		s, c, wire := eventFixture(t)
		s.Assets, s.World = catalog, world.New(catalog)
		c.character.Map = noaRecruitmentMap
		c.character.Pets = nil
		c.character.Quests = map[uint32]game.Quest{}
		saveBreillatFixture(t, s, c)
		ev, _ := s.World.Event(noaRecruitmentMap, noaRecruitmentEvent)
		branch := s.World.FindBranch(c.character, c.view, noaRecruitmentMap, ev, world.TriggerEntry, 0, 0, -1)
		if err := s.startEvent(context.Background(), c, noaRecruitmentActor, ev, branch, true); err != nil {
			t.Fatal(err)
		}
		for step := 0; c.event != nil && c.event.onChoice == nil && step < 10; step++ {
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
		}
		if c.event == nil || c.event.onChoice == nil {
			t.Fatal("quiz invitation missing")
		}
		if err := s.eventCommand(context.Background(), c, []byte{20, 9, answer}); err != nil {
			t.Fatal(err)
		}
		for step := 0; c.event != nil && step < 10; step++ {
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
		}
		if c.event != nil || len(c.character.Pets) != 0 || !contains(wire.packets(t), []byte{20, 8}) {
			t.Fatal("declining/cancelling did not end the conversation", answer)
		}
	}
}
