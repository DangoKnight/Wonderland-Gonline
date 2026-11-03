package server

import (
	"context"
	"reflect"
	"testing"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func battlePetFixture(t *testing.T) (*Server, *Session, *captureConn) {
	t.Helper()
	s, c, w := battleFixture(t, 10000)
	run := c.battle
	run.timer.Stop()
	next := c.character.Clone()
	next.Pets = []game.Pet{
		{ID: 10501, Slot: 1, Name: "Outgoing", Level: 1, Amity: 60, HP: 100, MaxHP: 100, SP: 30, MaxSP: 30, Battle: true},
		{ID: 10502, Slot: 2, Name: "Incoming", Level: 1, Amity: 70, HP: 75, MaxHP: 100, SP: 20, MaxSP: 30},
	}
	next.ActivePet = 10501
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, pet := range c.character.Pets {
		c.pets.register(pet.ID)
	}
	m := run.member(c)
	m.self.Char = c.character
	m.pet = battle.PetFighter(c.character.Pets[0], next.ID, s.Assets.Items, 0)
	m.pet.HP, m.pet.SP, m.pet.Deaths = 43, 7, 1
	run.b.Attackers = append(run.b.Attackers, m.pet,
		&battle.Fighter{Kind: battle.Player, Side: battle.Attacker, ID: 999, HP: 1000, MaxHP: 1000, X: 4, Y: 3})
	w.Reset()
	return s, c, w
}

func TestBattlePetSwitchCommitsVitalsAndConsumesTurn(t *testing.T) {
	s, c, w := battlePetFixture(t)
	run, m := c.battle, c.battle.member(c)
	old := m.pet
	// A pet-first command and a captured monster must not be executed/awarded by switching.
	run.b.Pending[3<<8|2] = battle.Action{Actor: old, Kind: "attack", Skill: 10001}
	run.b.Captures = []battle.Capture{{Owner: c.character.ID, Template: 10500, Name: "Captured", Level: 1}}
	m.choiceToken = 9
	if err := s.worldCommand(context.Background(), c, []byte{19, 1, 6, 41, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if m.pet == old || m.pet.ID != 10502 || m.pet.HP != 75 || m.pet.SP != 20 || !run.b.HasCommand(m.self) || !run.b.HasCommand(m.pet) || m.choiceToken != 0 || len(run.petChanges) != 2 {
		t.Fatal("switch did not consume both commands", m.pet, run.b.Pending)
	}
	for _, a := range run.b.Pending {
		if a.Kind != battle.ActionIdle {
			t.Fatal("switch attacked or defended", a)
		}
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored[0].Pets) != 2 || stored[0].ActivePet != 10502 || stored[0].Pets[0].HP != 43 || stored[0].Pets[0].SP != 7 || stored[0].Pets[0].Amity != 59 {
		t.Fatal("outgoing state not saved exactly once", stored, err)
	}
	packets := w.packets(t)
	if contains(packets, []byte{19, 1, 6, 41, 0, 0}) || !contains(packets, []byte{53, 5, 4, 2}) || contains(packets, []byte{11, 1, 3, 2}) {
		t.Fatal("selection receipt or delayed exit", packets)
	}
	// Next command phase: switching back restores saved vitals, not full health.
	run.b.Pending = map[int]battle.Action{}
	if err := s.worldCommand(context.Background(), c, []byte{19, 1, 5, 41, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if m.pet.ID != 10501 || m.pet.HP != 43 || m.pet.SP != 7 || c.character.Pets[0].Amity != 59 {
		t.Fatal("returning pet refilled or paid duplicate death penalty", m.pet)
	}
}

func TestBattlePetSwitchRejectionsAndSaveFailure(t *testing.T) {
	for _, name := range []string{"same", "foreign", "mounted", "low amity", "dead pet", "dead player", "submitted", "playing", "save failure"} {
		t.Run(name, func(t *testing.T) {
			s, c, w := battlePetFixture(t)
			run, m := c.battle, c.battle.member(c)
			request := []byte{19, 1, 6, 41, 0, 0}
			switch name {
			case "same":
				request[2] = 5
			case "foreign":
				request[2] = 7
			case "mounted":
				c.character.ActiveMount = 10502
			case "low amity":
				c.character.Pets[1].Amity = 39
			case "dead pet":
				c.character.Pets[1].HP = 0
			case "dead player":
				m.self.HP = 0
			case "submitted":
				run.b.Idle(m.self)
			case "playing":
				run.b.Processing = true
			case "save failure":
				s.Store.Close()
			}
			before, old := c.character.Clone(), m.pet
			err := s.worldCommand(context.Background(), c, request)
			if (err != nil) != (name == "save failure") || !reflect.DeepEqual(before, *c.character) || m.pet != old || len(run.petChanges) != 0 || w.Len() != 0 {
				t.Fatal("rejected switch changed state or published", err)
			}
		})
	}
}

func TestBattlePetRestAndMixedTransportPlayback(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "chain"}[bundled], func(t *testing.T) {
			s, c, w := battlePetFixture(t)
			run, m := c.battle, c.battle.member(c)
			if bundled {
				c.combatCapabilities = 1
			}
			if err := s.worldCommand(context.Background(), c, []byte{19, 2}); err != nil {
				t.Fatal(err)
			}
			if m.pet != nil || c.character.ActivePet != 0 || run.b.Expected() != 2 || len(run.petChanges) != 1 {
				t.Fatal("rest retained combat pet")
			}
			w.Reset()
			s.worldMu.Lock()
			run.b.Timeout()
			s.tryRound(run)
			s.worldMu.Unlock()
			settle(t, s, c)
			packets := w.packets(t)
			if bundled {
				steps := decodeRoundPackets(t, packets)
				if len(steps) == 0 || !contains(steps[0].Packets, []byte{11, 1, 3, 2}) || !contains(steps[0].Packets, []byte{19, 2}) {
					t.Fatal("chain lost pet exit", steps)
				}
			} else if !contains(packets, []byte{11, 1, 3, 2}) || !contains(packets, []byte{19, 2}) {
				t.Fatal("native lost pet exit", packets)
			}
			if run.b.Defenders[0].HP != 10000 {
				t.Fatal("rest performed attack")
			}
		})
	}
}

func TestBattlePetSwitchPlaybackAndNextPrompt(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "chain"}[bundled], func(t *testing.T) {
			s, c, w := battlePetFixture(t)
			run := c.battle
			observerWire := &captureConn{}
			observer := &Session{conn: observerWire, character: &game.Character{ID: 999}, battle: run}
			// Keep the other participant on the opposite transport to check
			// personalized receipts in an actual mixed-client battle.
			if !bundled {
				observer.combatCapabilities = 1
			}
			run.members = append(run.members, &battleMember{c: observer, self: run.b.Attackers[len(run.b.Attackers)-1]})
			if bundled {
				c.combatCapabilities = 1
			}
			if err := s.worldCommand(context.Background(), c, []byte{19, 1, 6, 41, 0, 0}); err != nil {
				t.Fatal(err)
			}
			w.Reset()
			s.worldMu.Lock()
			run.b.Timeout()
			s.tryRound(run)
			s.worldMu.Unlock()
			settle(t, s, c)
			packets := w.packets(t)
			observed := observerWire.packets(t)
			if !bundled {
				var flat [][]byte
				for _, step := range decodeRoundPackets(t, observed) {
					flat = append(flat, step.Packets...)
				}
				observed = flat
			}
			if contains(observed, []byte{19, 1, 6, 41, 0, 0}) || !contains(observed, []byte{11, 1, 3, 2}) {
				t.Fatal("selection leaked to another owner or exit was lost", observed)
			}
			var flat [][]byte
			if bundled {
				steps := decodeRoundPackets(t, packets)
				if len(steps) < 3 || !contains(steps[0].Packets, []byte{11, 1, 3, 2}) || !contains(steps[0].Packets, []byte{19, 1, 6, 41, 0, 0}) || len(steps[1].Packets) != 1 {
					t.Fatal("missing ordered exit/join preamble", steps)
				}
				for _, step := range steps {
					flat = append(flat, step.Packets...)
				}
			} else {
				flat = packets
			}
			exit, selected, join := -1, -1, -1
			for i, p := range flat {
				if len(p) == 4 && p[0] == 11 && p[1] == 1 && p[2] == 3 {
					exit = i
				}
				if reflect.DeepEqual(p, []byte{19, 1, 6, 41, 0, 0}) {
					selected = i
				}
				if len(p) == 34 && p[0] == 11 && p[1] == 5 && p[3] == 4 && p[4] == 6 && p[5] == 41 {
					join = i
					if p[2] != 5 || p[8] != 2 {
						t.Fatal("replacement lacks native waiting mode/owned slot", p)
					}
				}
			}
			if exit < 0 || selected <= exit || join <= selected || contains(flat, []byte{53, 5, 3, 2}) || run.b.Defenders[0].HP != 10000 {
				t.Fatal("switch attacked or lost native departure/replacement", flat)
			}
			s.worldMu.Lock()
			m := run.member(c)
			if run.b.HasCommand(m.self) || run.b.HasCommand(m.pet) || run.b.Processing || bundled && m.choiceToken == 0 {
				t.Error("next round remained consumed")
			}
			if m.pet.SP != 20 || m.pet.Joining {
				t.Error("switch spent pet SP")
			}
			if run.timer != nil {
				run.timer.Stop()
			}
			s.worldMu.Unlock()
		})
	}
}
