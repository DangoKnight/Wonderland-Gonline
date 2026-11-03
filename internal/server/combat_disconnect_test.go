package server

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func TestBattleDisconnectDuringResolvedRoundPersistsAndWithdraws(t *testing.T) {
	for _, chain := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "chain"}[chain], func(t *testing.T) {
			s, players, wires := teamBattle(t, 10000)
			a, b := players[0], players[1]
			run := a.battle
			run.timer.Stop()
			t.Cleanup(func() {
				if run.timer != nil {
					run.timer.Stop()
				}
			})
			m := run.member(a)
			next := a.character.Clone()
			next.Skills = []game.LearnedSkill{{ID: 11001, Grade: 1}}
			next.Pets = []game.Pet{{ID: 10501, Slot: 1, Level: 1, Name: "Pet", HP: 100, MaxHP: 100, SP: 30, MaxSP: 30, Amity: 60}}
			next.ActivePet = 10501
			if err := s.commit(context.Background(), a, next); err != nil {
				t.Fatal(err)
			}
			a.pets.register(10501)
			m.pet = battle.PetFighter(next.Pets[0], next.ID, s.Assets.Items, 0)
			m.pet.HP, m.pet.SP, m.pet.Deaths = 43, 7, 1
			run.b.Attackers = append(run.b.Attackers, m.pet)
			m.self.HP, m.self.SP, m.self.Spd = 100, 20, 100
			enemy := run.b.Defenders[0]
			enemy.Spd, enemy.Atk = 10000, 20
			rules := s.rules()
			rules.Next = func(lo, hi int) int { return lo }
			rules.Float = func() float64 { return 0 }
			rules.Skills = map[uint16]assets.Skill{11001: {ID: 11001, Name: "Fire Ball", SP: 10, EffectLayer: 2, StatMultiplier: 2}}
			if run.b.Submit(rules, a.character.ID, 1, []byte{4, 2, 2, 2, 249, 42}) == nil {
				t.Fatal("skill choice rejected")
			}
			run.b.Idle(m.pet)
			run.b.Idle(run.member(b).self)
			run.b.Processing = true
			steps, outcome := run.b.Round(rules)
			wantHP, wantSP := uint32(m.self.HP), uint32(m.self.SP)
			if wantHP >= 100 || wantSP != 10 || outcome != battle.Continue {
				t.Fatal("fixture did not calculate damage/SP before playback", wantHP, wantSP, outcome)
			}
			if chain {
				a.combatCapabilities = 1
				b.combatCapabilities = 1
			}
			for _, w := range wires {
				w.Reset()
			}
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			oldSleep := battleSleep
			var once sync.Once
			battleSleep = func(time.Duration) { once.Do(func() { close(entered); <-release }) }
			go func() { s.playRound(run, steps, outcome); close(done) }()
			var resumeOnce sync.Once
			resume := func() { resumeOnce.Do(func() { close(release) }); <-done; battleSleep = oldSleep }
			defer resume()
			<-entered
			// Native playback has only begun; a chain recipient already has every step.
			if chain {
				decodeRoundPackets(t, wires[1].packets(t))
			} else {
				wires[1].Reset()
			}
			s.leaveWorld(a)
			stored, err := s.Store.Characters(context.Background(), a.account.ID)
			valid := err == nil && stored[0].HP == wantHP && stored[0].SP == wantSP && stored[0].Pets[0].HP == 43 && stored[0].Pets[0].SP == 7 && stored[0].Pets[0].Amity == 59
			withdrawn := a.battle == nil && m.self.Withdrawn && m.pet.Withdrawn && b.battle == run && !run.b.Finished
			resume()
			packets := wires[1].packets(t)
			removal := contains(packets, []byte{11, 1, 4, 2, 0}) && contains(packets, []byte{11, 1, 3, 2, 0}) && contains(packets, protocol.Builder{11, 0}.U32(a.character.ID).U16(0))
			if !valid {
				t.Fatal("resolved player/pet consequences not saved", stored, err, wantHP, wantSP)
			}
			if !withdrawn || !removal {
				t.Fatal("disconnect did not remove owner/pet while teammate continued", withdrawn, packets)
			}
			if b.battle != run || run.b.Finished || run.b.Processing {
				t.Fatal("remaining player cannot continue")
			}
			before := stored[0]
			s.abandonBattle(a)
			stored, err = s.Store.Characters(context.Background(), a.account.ID)
			if err != nil || stored[0].HP != before.HP || stored[0].Pets[0].Amity != 59 {
				t.Fatal("duplicate disconnect settled twice", err)
			}
		})
	}
}

func TestBattleDisconnectAllPlayersStopsPlaybackAndSavesEach(t *testing.T) {
	s, players, _ := teamBattle(t, 10000)
	run := players[0].battle
	run.timer.Stop()
	run.b.Processing = true
	for i, c := range players[:2] {
		m := run.member(c)
		m.self.HP, m.self.SP = 40+i, 5+i
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	oldSleep := battleSleep
	var once sync.Once
	battleSleep = func(time.Duration) { once.Do(func() { close(entered); <-release }) }
	go func() {
		s.playRound(run, []battle.Step{{Packets: [][]byte{{50, 6, 4, 2, 0}}, Delay: time.Second}, {Packets: [][]byte{{50, 6, 4, 3, 0}}}}, battle.Continue)
		close(done)
	}()
	var resumeOnce sync.Once
	resume := func() { resumeOnce.Do(func() { close(release) }); <-done; battleSleep = oldSleep }
	defer resume()
	<-entered
	var disconnects sync.WaitGroup
	for _, c := range players[:2] {
		disconnects.Add(1)
		go func() { defer disconnects.Done(); s.leaveWorld(c) }()
	}
	disconnects.Wait()
	resume()
	if !run.b.Finished || len(run.active()) != 0 {
		t.Fatal("empty battle kept running")
	}
	for i, c := range players[:2] {
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || stored[0].HP != uint32(40+i) || stored[0].SP != uint32(5+i) || c.battle != nil {
			t.Fatal("concurrent disconnect lost state", i, stored, err)
		}
	}
}

func TestBattleDisconnectFailedSaveRetainsSnapshotForRetry(t *testing.T) {
	s, c, _ := battlePetFixture(t)
	run := c.battle
	m := run.member(c)
	m.self.HP, m.self.SP = 37, 9
	before := c.character.Clone()
	original := s.Store
	failed, err := store.Open(filepath.Join(t.TempDir(), "failed.db"))
	if err != nil {
		t.Fatal(err)
	}
	failed.Close()
	s.Store = failed
	s.abandonBattle(c)
	s.Store = original
	if c.battle != nil || c.battleDisconnect == nil || c.autosaveBaseline.HP != before.HP || c.character.HP != before.HP || c.character.Pets[0].Amity != 60 {
		t.Fatal("failed save adopted results or lost retry snapshot")
	}
	if err := s.autosaveSession(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].HP != 37 || stored[0].SP != 9 || stored[0].Pets[0].HP != 43 || stored[0].Pets[0].SP != 7 || stored[0].Pets[0].Amity != 59 || c.battleDisconnect != nil {
		t.Fatal("retry lost battle state", stored, err)
	}
	if err := s.autosaveSession(context.Background(), c); err != nil || c.character.Pets[0].Amity != 59 {
		t.Fatal("retry applied pet penalty twice", err)
	}
}

func TestBattleDisconnectAfterSavedResultDoesNotOverwriteOrRepeat(t *testing.T) {
	s, c, _ := battlePetFixture(t)
	run := c.battle
	entered, release, resumed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	oldSleep := battleSleep
	battleSleep = func(time.Duration) { close(entered); <-release; close(resumed) }
	defer func() { close(release); <-resumed; battleSleep = oldSleep }()
	s.worldMu.Lock()
	s.endBattle(run, battle.Defeat)
	s.worldMu.Unlock()
	<-entered // Durable defeat settlement precedes its departure animation.
	wantHP, wantSP := c.character.HP, c.character.SP
	s.leaveWorld(c)
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].HP != wantHP || stored[0].SP != wantSP || stored[0].Pets[0].Amity != 59 {
		t.Fatal("disconnect overwrote settled result", stored, err)
	}
}
