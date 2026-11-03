package server

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func decodeRoundPackets(t *testing.T, packets [][]byte) []protocol.CombatStep {
	t.Helper()
	var a protocol.CombatAssembly
	for _, p := range packets {
		if len(p) < 2 || p[0] != 254 || p[1] != 1 {
			continue
		}
		done, err := a.Add(p)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			steps, err := protocol.DecodeCombatRound(a.Data)
			if err != nil {
				t.Fatal(err)
			}
			return steps
		}
	}
	t.Fatal("missing full round", packets)
	return nil
}

func TestGonlineCombatNegotiation(t *testing.T) {
	s, c, w := battleFixture(t, 1000)
	startup := s.Config
	startup.CombatRoundChain = true
	s = New(startup, s.Store, s.Assets, s.Log)
	c.battle.timer.Stop()
	c.battle = nil
	w.Reset()
	accept := []byte{1, 241, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0}
	if err := s.dispatch(context.Background(), c, accept); err == nil || c.combatCapabilities != 0 {
		t.Fatal("unsolicited acceptance")
	}
	if err := s.discoveryCommand(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}
	packets := w.packets(t)
	if len(packets) != 3 || !bytes.Equal(packets[2], []byte{1, 240, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0}) {
		t.Fatal("offer", packets)
	}
	if err := s.dispatch(context.Background(), c, accept); err != nil || c.combatCapabilities != 1 {
		t.Fatal("negotiation", err)
	}
	if !contains(w.packets(t), []byte{1, 242, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0}) {
		t.Fatal("missing confirmation")
	}
	// The generic native handshake remains unchanged, even at extension subcodes.
	if err := s.dispatch(context.Background(), c, []byte{1, 241}); err != nil || !contains(w.packets(t), []byte{1, 241, 1}) {
		t.Fatal("native handshake changed")
	}
}

func TestGonlineCombatDisabledByDefault(t *testing.T) {
	s, c, w := battleFixture(t, 1000)
	c.battle.timer.Stop()
	c.battle = nil
	w.Reset()
	// Mutating the exported config cannot silently change the captured startup policy.
	s.Config.CombatRoundChain = true
	if err := s.discoveryCommand(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}
	packets := w.packets(t)
	if len(packets) != 2 || c.combatOffered || s.combatRoundChain {
		t.Fatal("default server offered full-round combat", packets)
	}
	c.combatOffered = true // Even a stale/forged offer cannot bypass the startup gate.
	if err := s.acceptCombatProtocol(c, []byte{1, 241, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0}); err == nil || c.combatCapabilities != 0 || w.Len() != 0 {
		t.Fatal("disabled extension was accepted or acknowledged", err)
	}
}

func TestGonlineCombatEnabledKeepsUnnegotiatedClientNative(t *testing.T) {
	s, c, w := battleFixture(t, 1000)
	run := c.battle
	run.timer.Stop()
	c.battle = nil
	startup := s.Config
	startup.CombatRoundChain = true
	s = New(startup, s.Store, s.Assets, s.Log)
	w.Reset()
	if err := s.discoveryCommand(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}
	if len(w.packets(t)) != 3 || !c.combatOffered || c.combatCapabilities != 0 {
		t.Fatal("offer activated transport without acceptance")
	}
	// A legacy-style short handshake reply cannot enable full-round delivery.
	if err := s.handshakeCommand(context.Background(), c, []byte{1, 240}); err != nil || c.combatCapabilities != 0 {
		t.Fatal("native handshake enabled extension", err)
	}
	w.Reset()
	c.battle = run
	if !s.play(run, []battle.Step{{Packets: [][]byte{{50, 6, 4, 2, 0}}}}) {
		t.Fatal("native playback stopped")
	}
	packets := w.packets(t)
	if !contains(packets, []byte{50, 6, 4, 2, 0}) {
		t.Fatal("native packet missing", packets)
	}
	for _, p := range packets {
		if p[0] == 254 {
			t.Fatal("unnegotiated client received a round envelope", packets)
		}
	}
}

func TestGonlineCombatSkillProgressSavedOnceWithFallback(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "chain", true: "native fallback"}[fallback], func(t *testing.T) {
			s, c, w := battleFixture(t, 1000)
			c.combatCapabilities = 1
			c.character.Skills = []game.LearnedSkill{{ID: 11001, Grade: 1, EXP: 99}}
			run := c.battle
			uses := []battle.Action{{Actor: run.member(c).self, Skill: 11001}}
			steps := []battle.Step{{Packets: [][]byte{{51, 1, 4, 2, 26, 9, 0, 0, 0}}, SkillUses: uses}, {Packets: [][]byte{{51, 1, 4, 2, 26, 8, 0, 0, 0}}, SkillUses: uses}}
			if fallback {
				steps[1].Delay = 2 * time.Minute
			}
			w.Reset()
			if !s.play(run, steps) {
				t.Fatal("round stopped")
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || stored[0].Skills[0].Grade != 2 || stored[0].Skills[0].EXP != 19 || c.character.Skills[0].EXP != 19 {
				t.Fatal("progress lost/repeated", stored, err)
			}
			packets := w.packets(t)
			if fallback {
				for _, p := range packets {
					if p[0] == 254 {
						t.Fatal("oversized timing sent as chain")
					}
				}
				if !contains(packets, steps[1].Packets[0]) {
					t.Fatal("fallback lost later step")
				}
			} else {
				decodeRoundPackets(t, packets)
			}
		})
	}
}

func TestGonlineCombatSaveFailurePublishesNoChain(t *testing.T) {
	s, c, w := battleFixture(t, 1000)
	c.combatCapabilities = 1
	c.character.Skills = []game.LearnedSkill{{ID: 11001, Grade: 1, EXP: 99}}
	run := c.battle
	w.Reset()
	s.Store.Close()
	s.play(run, []battle.Step{{Packets: [][]byte{{51, 1, 4, 2, 26, 9, 0, 0, 0}}, SkillUses: []battle.Action{{Actor: run.member(c).self, Skill: 11001}}}})
	if w.Len() != 0 || c.character.Skills[0].EXP != 99 || c.battle != nil {
		t.Fatal("failed save published/adopted chain")
	}
}

func TestGonlineCombatRequestExecutesRoundAndRenewsPrompt(t *testing.T) {
	s, c, w := battleFixture(t, 1000)
	c.combatCapabilities = 1
	run := c.battle
	m := run.member(c)
	prompt, err := s.combatPrompt(m, run.b.NextRound(c.character.ID))
	if err != nil || len(prompt) != 1 || !bytes.Equal(prompt[0], []byte{254, 3, 1, 0, 0, 0, 4, 2}) {
		t.Fatal("first prompt", prompt, err)
	}
	w.Reset()
	p := []byte{254, 2, 1, 0, 0, 0, 1, 50, 1, 4, 2, 2, 2, 17, 39}
	if err := s.worldCommand(context.Background(), c, p); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	if c.battle != run || run.b.Processing || m.choiceToken != 3 {
		t.Fatal("round did not renew prompt", m.choiceToken)
	}
	packets := w.packets(t)
	decodeRoundPackets(t, packets)
	if !contains(packets, []byte{254, 3, 3, 0, 0, 0, 4, 2}) || contains(packets, []byte{52, 1}) {
		t.Fatal("next prompt transport", packets)
	}
	if err := s.worldCommand(context.Background(), c, p); err != nil || len(run.b.Pending) != 0 || w.Len() != 0 {
		t.Fatal("completed round request replayed", err)
	}
}

func TestGonlineCombatMixedRoundSendsChainBeforeFirstWait(t *testing.T) {
	s, players, wires := teamBattle(t, 10000)
	run := players[0].battle
	players[0].combatCapabilities = 1
	for _, w := range wires {
		w.Reset()
	}
	steps := []battle.Step{{Packets: [][]byte{{50, 6, 4, 2, 0}, {51, 1, 2, 2, 25, 80, 0, 0, 0}}, Delay: 100 * time.Millisecond}, {Packets: [][]byte{{51, 1, 4, 2, 25, 70, 0, 0, 0}}, Delay: 200 * time.Millisecond}}
	waits := 0
	oldSleep := battleSleep
	defer func() { battleSleep = oldSleep }()
	battleSleep = func(time.Duration) {
		waits++
		if waits != 1 {
			return
		}
		chain := decodeRoundPackets(t, wires[0].packets(t))
		want := []protocol.CombatStep{{Packets: steps[0].Packets, Delay: steps[0].Delay}, {Packets: steps[1].Packets, Delay: steps[1].Delay}}
		if !reflect.DeepEqual(chain, want) {
			t.Fatal("different round", chain)
		}
		legacy := wires[1].packets(t)
		if !reflect.DeepEqual(legacy, steps[0].Packets) {
			t.Fatal("legacy timing changed", legacy)
		}
	}
	if !s.play(run, steps) || waits != 2 {
		t.Fatal("round stopped")
	}
	if packets := wires[0].packets(t); len(packets) != 0 {
		t.Fatal("duplicate individual delivery", packets)
	}
	if packets := wires[1].packets(t); !reflect.DeepEqual(packets, steps[1].Packets) {
		t.Fatal("legacy remainder", packets)
	}
}

func TestGonlineCombatChoicesAtomicOwnedAndReplaySafe(t *testing.T) {
	s, players, wires := teamBattle(t, 10000)
	c := players[0]
	c.combatCapabilities = 1
	run := c.battle
	m := run.member(c)
	m.pet = &battle.Fighter{Kind: battle.Pet, Side: battle.Attacker, ID: 10501, Owner: c.character.ID, X: 3, Y: 2, HP: 100, MaxHP: 100}
	run.b.Attackers = append(run.b.Attackers, m.pet)
	m.choiceToken = 9
	for _, w := range wires {
		w.Reset()
	}
	self := []byte{50, 1, 4, 2, 2, 2, 17, 39}
	pet := []byte{50, 4, 3, 2, 3, 2, 117, 234}
	foreign := []byte{50, 1, 4, 3, 2, 2, 17, 39}
	for _, choices := range [][][]byte{{self}, {self, self}, {self, foreign}} {
		p, _ := protocol.EncodeCombatChoices(9, choices)
		if err := s.worldCommand(context.Background(), c, p); err == nil || len(run.b.Pending) != 0 || wires[0].Len() != 0 {
			t.Fatal("non-atomic/foreign choices", choices, err)
		}
	}
	p, _ := protocol.EncodeCombatChoices(9, [][]byte{self, pet})
	if err := s.worldCommand(context.Background(), c, p); err != nil || len(run.b.Pending) != 2 || m.choiceToken != 0 || run.b.Processing {
		t.Fatal("batch not accepted together", err)
	}
	acks := wires[0].packets(t)
	if !contains(acks, []byte{53, 5, 4, 2}) || !contains(acks, []byte{53, 5, 3, 2}) {
		t.Fatal("native acknowledgments", acks)
	}
	clear(run.b.Pending)
	m.choiceToken = 10
	if err := s.worldCommand(context.Background(), c, p); err != nil || len(run.b.Pending) != 0 || wires[0].Len() != 0 {
		t.Fatal("old token replayed", err)
	}
}

func TestGonlineCombatOverkillTruncatesOwnerChain(t *testing.T) {
	s, players, wires := teamBattle(t, 10000)
	overkillDestinations(s)
	run := players[0].battle
	for _, c := range players[:2] {
		c.combatCapabilities = 1
	}
	for _, w := range wires {
		w.Reset()
	}
	marker := []byte{51, 1, 4, 3, 25, 77, 0, 0, 0}
	if !s.play(run, []battle.Step{overkillStep(run.member(players[0])), {Packets: [][]byte{marker}}}) {
		t.Fatal("survivor stopped")
	}
	owner := decodeRoundPackets(t, wires[0].packets(t))
	peer := decodeRoundPackets(t, wires[1].packets(t))
	if len(owner) != 1 || len(peer) != 3 || !contains(peer[2].Packets, marker) || players[0].character.Map != 11016 || players[1].battle != run {
		t.Fatal("overkill chain/return", owner, peer)
	}
	if !contains(peer[1].Packets, protocol.Builder{11, 0}.U32(players[0].character.ID).U16(0)) {
		t.Fatal("peer removal not scheduled")
	}
}
