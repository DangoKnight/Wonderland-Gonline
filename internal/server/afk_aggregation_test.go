package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestAFKAggregationPrivacyAndCarrierFallback(t *testing.T) {
	s, cs, ws := worldFixture(t)
	team := &party{members: cs}
	token := [16]byte{1}
	for _, c := range cs {
		c.ready = true
		c.party = team
		c.connectionNegotiated = true
		c.connectionFeatures = 96
		c.afk = true
		c.afkGroupToken = token
	}
	cs[0].character.Pets = []game.Pet{{ID: 100, HP: 10, MaxHP: 20}, {ID: 101, HP: 11, MaxHP: 21}}
	cs[0].character.ActivePet = 100
	now := time.Now()
	s.syncAFK(cs[0], now)
	stream := cs[0].afkStream
	if stream == nil {
		t.Fatal("no shared stream")
	}
	sharedCount := 0
	for i, w := range ws {
		for _, p := range w.packets(t) {
			if len(p) < 2 {
				continue
			}
			if p[0] == 253 && p[1] == 7 {
				sharedCount++
				if i != 0 {
					t.Fatal("duplicate common payload")
				}
				state, err := protocol.DecodeAFKShared(p)
				if err != nil {
					t.Fatal(err)
				}
				for _, u := range state.State.Units {
					if u.ID == 101 {
						t.Fatal("resting pet leaked")
					}
				}
			}
			if p[0] == 253 && p[1] == 2 {
				state, err := protocol.DecodeAFKState(p)
				if err != nil {
					t.Fatal(err)
				}
				if i != 0 || len(state.Units) != 1 || state.Units[0].ID != 101 {
					t.Fatal("private payload wrong", state)
				}
			}
		}
		w.Reset()
	}
	if sharedCount != 1 {
		t.Fatal("expected one stream", sharedCount)
	}
	for _, c := range cs {
		s.syncAFK(c, now.Add(2*time.Second))
	}
	for _, w := range ws {
		for _, p := range w.packets(t) {
			if p[0] == 253 && p[1] == 7 {
				t.Fatal("unchanged shared state resent")
			}
		}
		w.Reset()
	}
	// Identical gameplay mutation: compare full independent packets against the
	// actual aggregate wire output, rather than differently timed battle runs.
	cs[0].character.HP--
	independentBytes := 0
	for _, c := range cs {
		p, err := protocol.EncodeAFKState(s.afkSnapshot(c, false))
		if err != nil {
			t.Fatal(err)
		}
		independentBytes += len(p)
		s.syncAFK(c, now.Add(4*time.Second))
	}
	aggregateBytes := 0
	for _, w := range ws {
		for _, p := range w.packets(t) {
			aggregateBytes += len(p)
		}
		w.Reset()
	}
	if aggregateBytes == 0 || aggregateBytes*2 >= independentBytes {
		t.Fatal("duplicate party stats were not eliminated", aggregateBytes, independentBytes)
	}
	t.Logf("matching state update: %d aggregate bytes versus %d independent bytes", aggregateBytes, independentBytes)
	cs[0].afk = false
	s.syncAFK(cs[1], now.Add(6*time.Second))
	if cs[1].afkStream == nil || cs[1].afkStream == stream || cs[1].afkStream.members[0] != cs[1] {
		t.Fatal("carrier not reassigned")
	}
	cs[2].afk = false
	ws[1].Reset()
	s.syncAFK(cs[1], now.Add(8*time.Second))
	if cs[1].afkStream != nil {
		t.Fatal("singleton retained stream")
	}
	found := false
	for _, p := range ws[1].packets(t) {
		if p[0] == 253 && p[1] == 2 {
			state, err := protocol.DecodeAFKState(p)
			if err != nil {
				t.Fatal(err)
			}
			found = len(state.Units) >= 3
		}
	}
	if !found {
		t.Fatal("independent snapshot not restored")
	}
}

func TestAFKAggregationRequiresMatchingConsentAndParty(t *testing.T) {
	s, cs, _ := worldFixture(t)
	for i, c := range cs {
		c.afk = true
		c.ready = true
		c.connectionNegotiated = true
		c.connectionFeatures = 96
		c.afkGroupToken = [16]byte{byte(i + 1)}
	}
	cs[0].party = &party{members: cs[:2]}
	cs[1].party = cs[0].party
	if s.ensureAFKStream(cs[0]) != nil {
		t.Fatal("different workspace tokens grouped")
	}
	cs[2].afkGroupToken = cs[0].afkGroupToken
	if s.ensureAFKStream(cs[0]) != nil {
		t.Fatal("unrelated party grouped")
	}
	c := cs[0]
	c.afk = false
	c.connectionFeatures = 32
	if err := s.afkCommand(context.Background(), c, append([]byte{253, 1, 1}, c.afkGroupToken[:]...)); err == nil {
		t.Fatal("unnegotiated extension accepted")
	}
}
