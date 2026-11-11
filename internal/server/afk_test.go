package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestAFKRequiresNegotiationAndIdleInteraction(t *testing.T) {
	s, cs, ws := worldFixture(t)
	c := cs[0]
	ctx := context.Background()
	if err := s.dispatch(ctx, c, []byte{253, 1, 1}); err == nil {
		t.Fatal("legacy enabled AFK")
	}
	c.connectionNegotiated = true
	c.connectionFeatures = 32
	c.ready = true
	c.event = &eventSession{}
	if err := s.dispatch(ctx, c, []byte{253, 1, 1}); err != nil {
		t.Fatal(err)
	}
	if c.afk || !contains(ws[0].packets(t), []byte{253, 5}) {
		t.Fatal("interaction was not rejected")
	}
	c.event = nil
	for _, p := range [][]byte{{253}, {253, 1}, {253, 1, 2}, {253, 1, 1, 0}, {253, 2, 1}} {
		if err := s.dispatch(ctx, c, p); err == nil {
			t.Fatal("accepted malformed request", p)
		}
	}
	if err := s.dispatch(ctx, c, []byte{253, 1, 1}); err != nil || !c.afk {
		t.Fatal("valid entry", err)
	}
	if !contains(ws[0].packets(t), []byte{253, 1, 1}) {
		t.Fatal("no mode acknowledgment")
	}
	if cs[1].afk || cs[1].afkFilter.Load() != nil {
		t.Fatal("AFK leaked to another session")
	}
}

func TestAFKFilteringResyncAndPartyPrivacy(t *testing.T) {
	s, cs, ws := worldFixture(t)
	c, peer, outsider := cs[0], cs[1], cs[2]
	ctx := context.Background()
	for _, m := range cs {
		m.ready = true
		s.world[m.info.ID] = m
	}
	c.connectionNegotiated = true
	c.connectionFeatures = 32
	c.party = &party{members: []*Session{c, peer}}
	peer.party = c.party
	peer.character.Pets = []game.Pet{{ID: 100, Name: "Pet", HP: 10, MaxHP: 20}}
	peer.character.ActivePet = 100
	if err := s.dispatch(ctx, c, []byte{253, 1, 1}); err != nil {
		t.Fatal(err)
	}
	packets := ws[0].packets(t)
	state, err := protocol.DecodeAFKState(packets[len(packets)-1])
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Units) != 3 {
		t.Fatal("missing party player/pet", state.Units)
	}
	for _, u := range state.Units {
		if u.Owner == outsider.character.ID {
			t.Fatal("disclosed unrelated stats")
		}
	}
	ws[0].Reset()
	ws[1].Reset()
	other := outsider.character.PositionPacket()
	if err := c.send(other); err != nil {
		t.Fatal(err)
	}
	if ws[0].Len() != 0 {
		t.Fatal("unrelated peer packet sent to AFK")
	}
	if err := peer.send(other); err != nil || ws[1].Len() == 0 {
		t.Fatal("normal connection filtered", err)
	}
	partyPosition := peer.character.PositionPacket()
	if err := c.send(partyPosition); err != nil || !contains(ws[0].packets(t), partyPosition) {
		t.Fatal("party movement lost", err)
	}
	ws[0].Reset()
	s.syncAFK(c, time.Now().Add(2*time.Second))
	if ws[0].Len() != 0 {
		t.Fatal("unchanged state resent")
	}
	peer.character.HP--
	s.syncAFK(c, time.Now().Add(4*time.Second))
	if ws[0].Len() == 0 {
		t.Fatal("changed party vitals missing")
	}
	ws[0].Reset()
	ws[1].Reset()
	// Leaving AFK is allowed even if an event started in the meantime.
	c.event = &eventSession{}
	if err := s.dispatch(ctx, c, []byte{253, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if c.afk || c.afkFilter.Load() != nil || c.event == nil {
		t.Fatal("exit changed gameplay ownership")
	}
	packets = ws[0].packets(t)
	if !bytes.Equal(packets[0], []byte{253, 3}) || !bytes.Equal(packets[len(packets)-1], []byte{253, 4}) {
		t.Fatal("refresh fences missing", packets)
	}
	if !contains(packets, peer.character.PositionPacket()) {
		t.Fatal("peer refresh missing")
	}
	if ws[1].Len() != 0 {
		t.Fatal("AFK exit rebroadcast a team join or map arrival")
	}
	if contains(packets, []byte{20, 8}) {
		t.Fatal("AFK exit released an event")
	}
}
