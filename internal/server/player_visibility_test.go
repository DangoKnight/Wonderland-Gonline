package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func TestStarterScenesHidePlayerSnapshotsAndBroadcasts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mapID, scene uint16
		private      bool
	}{
		{"ship", 10017, 10001, true},
		{"cabin", 10027, 10002, true},
		{"island", 10035, 10003, true},
		{"public", 11016, 11016, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, w := worldFixture(t)
			for _, c := range p[:2] {
				c.character.Map = tc.mapID
			}
			s.Assets.Maps[tc.mapID] = assets.Map{ID: tc.mapID, Scene: tc.scene}
			s.World = world.New(s.Assets)
			ctx := context.Background()
			if err := s.acknowledgeWorld(p[0]); err != nil {
				t.Fatal(err)
			}
			w[0].Reset()
			// A tent sign must not bypass the normal peer snapshot filter.
			p[0].openTent = &openTent{Map: tc.mapID, X: 1000, Y: 1000, Slot: 1}
			if err := s.acknowledgeWorld(p[1]); err != nil {
				t.Fatal(err)
			}
			arrival := w[0].packets(t)
			snapshot := w[1].packets(t)
			if tc.private {
				if len(arrival) != 0 || countPrefix(snapshot, 4) != 0 || hasPacket(snapshot, tentSign(p[0])) {
					t.Fatal("private entry exposed a peer", arrival, snapshot)
				}
				if s.mapPlayer(p[0], p[1].character.ID) != nil {
					t.Fatal("hidden player discoverable by local commands")
				}
			} else if countPrefix(arrival, 4) != 1 || countPrefix(snapshot, 4) != 1 || !hasPacket(snapshot, tentSign(p[0])) {
				t.Fatal("public entry lost peers", arrival, snapshot)
			}
			if tc.private {
				if err := s.enterTent(ctx, p[1], p[0].character.ID); err != nil {
					t.Fatal(err)
				}
				if p[1].tentOwner != 0 || p[1].character.Map != tc.mapID {
					t.Fatal("entered hidden player's tent")
				}
				w[1].Reset()
			}
			for _, packet := range [][]byte{
				protocol.Builder{6, 1}.U32(p[0].character.ID).U8(2).U16(1200).U16(1300),
				protocol.Builder{32, 1}.U32(p[0].character.ID).U8(1),
				protocol.Builder{11, 4}.U32(p[0].character.ID).U8(1),
				protocol.Builder{15, 4}.U32(p[0].character.ID).U32(12178),
			} {
				s.broadcastWorld(p[0], packet)
				got := w[1].packets(t)
				if tc.private && len(got) != 0 || !tc.private && (len(got) != 1 || !bytes.Equal(got[0], packet)) {
					t.Fatal("broadcast visibility", got)
				}
			}
			p[0].party = &party{members: []*Session{p[0], p[1]}}
			p[1].party = p[0].party
			s.partyArrival(p[0])
			if tc.private && countPrefix(w[1].packets(t), 13, 5) != 0 {
				t.Fatal("party formation exposed a peer")
			}
			if tc.private && len(s.battleTeam(p[0])) != 1 {
				t.Fatal("hidden teammate entered personal battle")
			}
			w[0].Reset()
			w[1].Reset()
			s.sendMap(tc.mapID, []byte{13, 5, 1, 0, 0, 0, 2, 0, 0, 0}, nil)
			if tc.private && (w[0].Len() != 0 || w[1].Len() != 0) {
				t.Fatal("map-wide formation exposed peers")
			}
			w[0].Reset()
			w[1].Reset()
			if err := s.closePlayerTent(ctx, p[0]); err != nil {
				t.Fatal(err)
			}
			if tc.private && w[1].Len() != 0 {
				t.Fatal("tent close exposed its owner")
			}
			w[0].Reset()
			w[1].Reset()
			s.leaveWorld(p[0])
			if tc.private && countPrefix(w[1].packets(t), 12) != 0 {
				t.Fatal("logout exposed a peer")
			}
		})
	}
}

func TestStarterPlayerPolicyPreservesSharedTent(t *testing.T) {
	s := &Server{World: world.New(&assets.Catalog{Maps: map[uint16]assets.Map{10017: {ID: 10017, Scene: 10001}}})}
	a := &Session{character: &game.Character{ID: 1, Map: 10017}, tentOwner: 1}
	b := &Session{character: &game.Character{ID: 2, Map: 10017}, tentOwner: 1}
	if !s.samePlayerScene(a, b) {
		t.Fatal("shared tent occupants hidden")
	}
	b.tentOwner = 2
	if s.samePlayerScene(a, b) {
		t.Fatal("different tent owners share visibility")
	}
	a.tentOwner = 0
	b.tentOwner = 0
	if s.samePlayerScene(a, b) || !s.samePlayerScene(a, a) {
		t.Fatal("starter visibility does not isolate other players")
	}
}

func TestStarterVisibilityChangesAtMapTransitions(t *testing.T) {
	s, players, wires := worldFixture(t)
	a, b := players[0], players[1]
	s.Assets.Maps[10017] = assets.Map{ID: 10017, Scene: 10001}
	s.Assets.Maps[20000] = assets.Map{ID: 20000, Scene: 20000}
	s.World = world.New(s.Assets)
	ctx := context.Background()
	for _, c := range []*Session{a, b} {
		if err := s.acknowledgeWorld(c); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range wires {
		w.Reset()
	}
	warp := func(c *Session, mapID uint16) {
		t.Helper()
		s.worldMu.Lock()
		err := s.teleport(ctx, c, world.Destination{Map: mapID, X: 1042, Y: 1075}, 0)
		s.worldMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if err = s.acknowledgeWorld(c); err != nil {
			t.Fatal(err)
		}
	}
	warp(a, 20000)
	for _, w := range wires {
		w.Reset()
	}
	warp(b, 20000)
	if countPrefix(wires[0].packets(t), 4) != 1 || countPrefix(wires[1].packets(t), 4) != 1 {
		t.Fatal("public map did not restore peer appearances")
	}
	warp(a, 10017)
	departure := wires[1].packets(t)
	if countPrefix(departure, 12) != 1 || countPrefix(departure, 4) != 0 {
		t.Fatal("public departure did not remove player", departure)
	}
	wires[0].Reset()
	wires[1].Reset()
	warp(b, 10017)
	if countPrefix(wires[0].packets(t), 4) != 0 || countPrefix(wires[1].packets(t), 4) != 0 {
		t.Fatal("return to private map exposed a player")
	}
}
