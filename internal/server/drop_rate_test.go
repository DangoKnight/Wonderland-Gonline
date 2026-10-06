package server

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestGMDropRateCommand(t *testing.T) {
	s, players, wires := chatFixture(t)
	gm := players[0]
	say(t, s, gm, "/droprate 100")
	if s.dropRateMultiplier != 0 || wires[0].Len() != 0 {
		t.Fatal("non-GM changed multiplier")
	}
	s.SetGMLevel(gm.account.ID, 1)
	say(t, s, gm, "/droprate")
	// Independent native head-banner layout; setting feedback is private.
	want := append([]byte{2, 16, 0, 0, 0, 0}, []byte("Current drop rate multiplier: 1.0x. Usage: /droprate <multiplier>")...)
	if packets := wires[0].packets(t); len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatal(packets)
	}
	for _, test := range []struct {
		input string
		want  float64
	}{
		{":droprate 2.5", 2.5}, {"/droprate 0", .1}, {"/droprate 101", 100},
		{"/droprate NaN", 100}, {"/droprate +Inf", 100}, {"/droprate 1e999", 100},
		{"/droprate invalid", 100}, {"/droprate 1 2", 100},
	} {
		say(t, s, gm, test.input)
		if s.dropRateMultiplier != test.want {
			t.Fatal(test.input, s.dropRateMultiplier)
		}
		if len(wires[0].packets(t)) != 1 {
			t.Fatal("missing feedback", test.input)
		}
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("private feedback broadcast")
	}
	s.SetGMLevel(gm.account.ID, 0)
	say(t, s, gm, "/droprate 1")
	if s.dropRateMultiplier != 100 {
		t.Fatal("revoked GM changed multiplier")
	}
}

func TestLiveDropRateReachesVictoryLoot(t *testing.T) {
	s, players, _ := chatFixture(t)
	s.SetGMLevel(players[0].account.ID, 1)
	say(t, s, players[0], "/droprate 100")
	const monsterID = 999
	s.Assets.NPCs = map[uint16]assets.NPC{monsterID: {Drops: [5]uint16{32176}}}
	s.Assets.Drops = map[uint32][]assets.Drop{monsterID: {{Item: 32176, Min: 1, Max: 1, Rate: 10}}}
	run := &battleRun{b: &battle.Battle{Defenders: []*battle.Fighter{{Kind: battle.Monster, Template: monsterID}, {Kind: battle.Monster, Template: monsterID, Captured: true}}}}
	next := players[0].character.Clone()
	next.Bag = game.Inventory{}
	s.worldMu.Lock()
	adds, _ := s.rollLoot(run, &next)
	s.worldMu.Unlock()
	if len(adds) != 1 || next.Bag[0].ID != 32176 || next.Bag[0].Count != 1 {
		t.Fatal("multiplier/capture restriction not applied", adds, next.Bag[0])
	}
}
