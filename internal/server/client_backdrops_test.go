package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
)

func TestClientBackdropsNegotiatedWithoutRoundChains(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	accept := []byte{1, 249, 71, 76, 79, 78, 1, 0, 5, 0, 0, 0}
	if err := s.handshakeCommand(context.Background(), c, accept); err == nil {
		t.Fatal("accepted unsolicited capability")
	}
	if err := s.discoveryCommand(context.Background(), c, nil); err != nil {
		t.Fatal(err)
	}
	// Include the AFK feature family while keeping round chains and agent/API
	// access disabled. Keep this raw offer independent of production constants.
	if !contains(wires[0].packets(t), []byte{1, 248, 71, 76, 79, 78, 1, 0, 253, 0, 0, 0}) {
		t.Fatal("missing backdrop offer")
	}
	if err := s.handshakeCommand(context.Background(), c, accept); err != nil {
		t.Fatal(err)
	}
	if !c.clientBackdrops || c.combatCapabilities != 0 || s.combatRoundChain {
		t.Fatal("backdrops depend on round chains")
	}
	if !contains(wires[0].packets(t), []byte{1, 250, 71, 76, 79, 78, 1, 0, 5, 0, 0, 0}) {
		t.Fatal("missing confirmation")
	}
	c.character.Map, c.character.X, c.character.Y = 60001, 2202, 1395
	s.Assets.BattleBackdrops = map[uint16]assets.BattleBackdrop{60001: {Default: 91}}
	run := &battleRun{members: []*battleMember{{c: c}}}
	if got := s.legacyBattleBackground(c, run); got != 1 {
		t.Fatal("server resolved native regions for a Gonline-only battle", got)
	}
	c.clientBackdrops = false
	if got := s.legacyBattleBackground(c, run); got != 91 {
		t.Fatal("legacy selection changed", got)
	}
	c.clientBackdrops = true
	run.members = append(run.members, &battleMember{c: players[1]})
	if got := s.legacyBattleBackground(c, run); got != 91 {
		t.Fatal("mixed group lost Legacy selection", got)
	}
	players[1].clientBackdrops = true
	if got := s.legacyBattleBackground(c, run); got != 1 {
		t.Fatal("all-Gonline group performed server resolution", got)
	}
}

func TestClientBackdropFormationPackets(t *testing.T) {
	s, c, wire := battleFixture(t, 1000)
	run := c.battle
	run.timer.Stop()
	run.backdropLocation = protocol.BattleBackdropLocation{Map: 60001, X: 2202, Y: 1395}
	for _, local := range []bool{false, true} {
		wire.Reset()
		c.clientBackdrops = local
		if err := s.battleIntro(run, run.members[0], 91); err != nil {
			t.Fatal(err)
		}
		found, contextBeforeFormation := false, false
		for _, p := range wire.packets(t) {
			if _, ok := protocol.ParseBattleBackdropLocation(p); ok {
				contextBeforeFormation = true
			}
			if len(p) > 3 && p[0] == 11 && p[1] == 250 {
				found = true
				want := byte(91)
				if local {
					want = 1
				}
				if p[2] != want || contextBeforeFormation != local {
					t.Fatal("wrong recipient backdrop path", local, p[:4], contextBeforeFormation)
				}
			}
		}
		if !found {
			t.Fatal("formation not sent")
		}
	}
}
