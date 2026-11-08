package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestKnownPlayerPresenceAcrossMaps(t *testing.T) {
	s, players, wires := worldFixture(t)
	a, b, stranger := players[0], players[1], players[2]
	for _, c := range []*Session{a, b, stranger} {
		if err := s.acknowledgeWorld(c); err != nil {
			t.Fatal(err)
		}
	}
	if !a.knowsPlayer(b.character.ID) || stranger.knowsPlayer(b.character.ID) {
		t.Fatal("appearance disclosure not tracked")
	}
	if err := s.commandTeleport(context.Background(), b, world.Destination{Map: 30000, X: 500, Y: 500}); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	s.leaveWorld(b)
	offline := protocol.Builder{12}.U32(b.character.ID).U16(0).U16(0).U16(0).U16(0).U8(0)
	if !contains(wires[0].packets(t), offline) {
		t.Fatal("known remote speaker logout omitted")
	}
	if wires[2].Len() != 0 {
		t.Fatal("unknown player disclosed by logout")
	}
	b.warped = false
	if err := s.acknowledgeWorld(b); err != nil {
		t.Fatal(err)
	}
	appearance, err := b.character.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(wires[0].packets(t), appearance) {
		t.Fatal("known remote speaker reconnect omitted")
	}
	if wires[2].Len() != 0 {
		t.Fatal("unknown player disclosed by reconnect")
	}
}

func TestKnownPlayerPresenceDoesNotSpawnAcrossPrivateInstances(t *testing.T) {
	s, players, wires := worldFixture(t)
	a, b := players[0], players[1]
	appearance, err := b.character.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.send(appearance); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	a.tentOwner, b.tentOwner = 1, 2
	s.friendSessions = map[uint32]*Session{a.character.ID: a, b.character.ID: b}
	s.knownPlayerOnline(b)
	packets := wires[0].packets(t)
	if !contains(packets, protocol.Builder{10, 3}.U32(b.character.ID).U8(255)) {
		t.Fatal("private-instance presence missing")
	}
	for _, p := range packets {
		if len(p) > 0 && p[0] == 4 {
			t.Fatal("private-instance character spawned through presence")
		}
	}
	wires[0].Reset()
	b.invisible = true
	s.knownPlayerOnline(b)
	if wires[0].Len() != 0 {
		t.Fatal("invisible character disclosed")
	}
}
