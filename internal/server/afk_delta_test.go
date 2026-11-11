package server

import (
	"testing"
	"time"
	"wonderland-gonline/internal/protocol"
)

func TestAFKDeltaNegotiationAndSharedBase(t *testing.T) {
	s, cs, ws := worldFixture(t)
	now := time.Now()
	for _, c := range cs {
		c.afk = true
		c.ready = true
		c.connectionNegotiated = true
		c.connectionFeatures = 224
		c.afkGroupToken = [16]byte{1}
		c.party = &party{members: cs}
	}
	for _, c := range cs {
		c.party = cs[0].party
	}
	s.syncAFK(cs[0], now)
	var base protocol.AFKState
	for _, p := range ws[0].packets(t) {
		if p[0] == 253 && p[1] == 7 {
			state, err := protocol.DecodeAFKShared(p)
			if err != nil {
				t.Fatal(err)
			}
			base = state.State
		}
	}
	if len(base.Units) != 3 {
		t.Fatal("initial common snapshot missing")
	}
	for _, w := range ws {
		w.Reset()
	}
	cs[1].character.HP--
	s.syncAFK(cs[0], now.Add(2*time.Second))
	packets := ws[0].packets(t)
	if len(packets) != 1 {
		t.Fatal(packets)
	}
	stream, seq, update, err := protocol.DecodeAFKSharedUpdate(packets[0])
	if err != nil || stream == 0 || seq != 2 || update[1] != 8 {
		t.Fatal(stream, seq, update, err)
	}
	next, err := protocol.ApplyAFKDelta(base, update)
	if err != nil || next.Units[1].HP != cs[1].character.HP {
		t.Fatal(next, err)
	}
	// A mixed-version group must still deliver full snapshots to its carrier.
	cs[2].connectionFeatures = 96
	cs[1].character.HP--
	ws[0].Reset()
	s.syncAFK(cs[0], now.Add(4*time.Second))
	for _, p := range ws[0].packets(t) {
		if p[0] == 253 && p[1] == 7 {
			if _, err := protocol.DecodeAFKShared(p); err != nil {
				t.Fatal("old subscriber given a delta", err)
			}
		}
	}
}
