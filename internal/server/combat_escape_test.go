package server

import (
	"context"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestEscapeOwnerAndPetMixedTransport(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		for _, petActs := range []bool{false, true} {
			for _, leaderEscapes := range []bool{false, true} {
				s, players, wires := teamBattle(t, 10000)
				leader, owner := players[0], players[1]
				if leaderEscapes {
					owner = leader
				}
				run := leader.battle
				run.timer.Stop()
				m := run.member(owner)
				next := owner.character.Clone()
				next.Pets = []game.Pet{{ID: 10501, Slot: 1, Name: "Runner", Level: 1, Amity: 60, HP: 43, MaxHP: 100, SP: 7, MaxSP: 30, Battle: true}}
				next.ActivePet = 10501
				if err := s.commit(context.Background(), owner, next); err != nil {
					t.Fatal(err)
				}
				owner.pets.register(10501)
				m.self.Char = owner.character
				m.self.HP, m.self.SP, m.self.Level = 80, 20, 50
				m.pet = battle.PetFighter(owner.character.Pets[0], owner.character.ID, s.Assets.Items, 1)
				m.pet.X, m.pet.Y = m.self.X-1, m.self.Y
				m.pet.HP, m.pet.SP, m.pet.Level = 30, 4, 50
				run.b.Attackers = append(run.b.Attackers, m.pet)
				if bundled {
					owner.combatCapabilities = 1
				} else {
					for _, other := range run.active() {
						if other.c != owner {
							other.c.combatCapabilities = 1
						}
					}
				}
				actor := m.self
				if petActs {
					actor = m.pet
				}
				for _, other := range run.active() {
					if other.self != actor {
						run.b.Idle(other.self)
					}
					if other.pet != actor {
						run.b.Idle(other.pet)
					}
				}
				for _, w := range wires {
					w.Reset()
				}
				request := []byte{50, 5, actor.X, actor.Y, actor.X, actor.Y, 137, 234}
				if err := s.worldCommand(context.Background(), owner, request); err != nil {
					t.Fatal(err)
				}
				settle(t, s, leader)
				settle(t, s, owner)
				if owner.battle != nil || (!leaderEscapes && (leader.battle != run || run.b.Finished)) || (leaderEscapes && players[1].battle != nil) {
					t.Fatal("escape scope", bundled, petActs, leaderEscapes)
				}
				stored, err := s.Store.Characters(context.Background(), owner.account.ID)
				if err != nil || stored[0].HP != 80 || stored[0].SP != 20 || stored[0].Pets[0].HP != 30 || stored[0].Pets[0].SP != 4 {
					t.Fatal("escape vitals not committed", stored, err)
				}
				if !leaderEscapes {
					ownerPackets := wires[1].packets(t)
					if bundled {
						var flat [][]byte
						for _, step := range decodeRoundPackets(t, ownerPackets) {
							flat = append(flat, step.Packets...)
						}
						ownerPackets = append(ownerPackets, flat...)
					}
					if !contains(ownerPackets, []byte{11, 1, m.self.X, m.self.Y}) || !contains(ownerPackets, []byte{11, 1, m.pet.X, m.pet.Y}) || !contains(ownerPackets, protocol.Builder{11, 0}.U32(m.self.ID).U16(0)) {
						t.Fatal("owner/pet did not leave together", ownerPackets)
					}
					for _, p := range ownerPackets {
						if len(p) > 1 && p[0] == 50 && p[1] == 1 {
							t.Fatal("escaper received later attacks")
						}
					}
					if run.timer != nil {
						run.timer.Stop()
					}
				}
			}
		}
	}
}

func TestEscapeFailedSavePublishesNoSuccess(t *testing.T) {
	s, players, wires := teamBattle(t, 10000)
	run := players[0].battle
	run.timer.Stop()
	m := run.member(players[1])
	m.self.Level = 50
	run.b.Idle(run.member(players[0]).self)
	run.b.Pending[int(m.self.X)<<8|int(m.self.Y)] = battle.Action{Actor: m.self, Kind: "flee"}
	steps, _ := run.b.Round(s.rules())
	for _, w := range wires {
		w.Reset()
	}
	s.Store.Close()
	s.play(run, steps)
	if wires[1].Len() != 0 || players[1].battle != nil || players[0].battle != run || run.b.Finished {
		t.Fatal("failed escape save published success or ended teammate battle")
	}
}

func TestEscapeFailureNativeAndRoundChain(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		s, players, wires := teamBattle(t, 10000)
		run := players[0].battle
		run.timer.Stop()
		m := run.member(players[1])
		if bundled {
			m.c.combatCapabilities = 1
		}
		m.self.Level = 1
		run.b.Defenders[0].Level = 60
		run.b.Pending[int(m.self.X)<<8|int(m.self.Y)] = battle.Action{Actor: m.self, Kind: "flee"}
		run.b.Idle(run.member(players[0]).self)
		r := s.rules()
		r.Next = func(lo, hi int) int { return lo }
		steps, outcome := run.b.Round(r)
		for _, w := range wires {
			w.Reset()
		}
		s.playRound(run, steps, outcome)
		packets := wires[1].packets(t)
		if bundled {
			var flat [][]byte
			for _, step := range decodeRoundPackets(t, packets) {
				flat = append(flat, step.Packets...)
			}
			packets = flat
		}
		want := []byte{50, 1, 11, 0, m.self.X, m.self.Y, 138, 234, 0, 1, m.self.X, m.self.Y, 1, 0, 0}
		if !contains(packets, want) || players[0].battle != run || players[1].battle != run || run.b.Finished || run.b.Processing {
			t.Fatal("failure animation or next turn lost", bundled, packets)
		}
		if run.timer != nil {
			run.timer.Stop()
		}
	}
}

func TestEscapeDepartureRacesDisconnect(t *testing.T) {
	for _, leaderDisconnects := range []bool{false, true} {
		s, players, _ := teamBattle(t, 10000)
		run := players[0].battle
		run.timer.Stop()
		m := run.member(players[1])
		m.self.Level = 50
		m.self.HP, m.self.SP = 30, 11
		run.b.Pending[int(m.self.X)<<8|int(m.self.Y)] = battle.Action{Actor: m.self, Kind: "flee"}
		run.b.Idle(run.member(players[0]).self)
		steps, outcome := run.b.Round(s.rules())
		run.b.Processing = true
		entered, release := make(chan struct{}), make(chan struct{})
		oldSleep := battleSleep
		var once sync.Once
		battleSleep = func(time.Duration) { once.Do(func() { close(entered); <-release }) }
		done := make(chan struct{})
		go func() { s.playRound(run, steps, outcome); close(done) }()
		<-entered
		s.worldMu.Lock()
		if leaderDisconnects {
			s.abandonBattle(players[0])
		} else {
			s.abandonBattle(players[1])
		}
		s.worldMu.Unlock()
		close(release)
		<-done
		// If a round schedules a separate ending, join it before restoring the
		// animation clock or reading state outside the owning world mutex.
		s.worldMu.Lock()
		finished := run.b.Finished
		s.worldMu.Unlock()
		if finished {
			for _, c := range players[:2] {
				settle(t, s, c)
			}
		}
		battleSleep = oldSleep
		if players[1].battle != nil || leaderDisconnects && !run.b.Finished || !leaderDisconnects && (players[0].battle != run || run.b.Finished || run.b.Processing) {
			t.Fatal("disconnect/escape race changed escape scope", leaderDisconnects)
		}
		stored, err := s.Store.Characters(context.Background(), players[1].account.ID)
		if err != nil || stored[0].HP != 30 || stored[0].SP != 11 {
			t.Fatal("disconnect replaced committed escape with a defeat", stored, err)
		}
		if run.timer != nil {
			run.timer.Stop()
		}
	}
}
