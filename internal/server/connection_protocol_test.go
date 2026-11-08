package server

import (
	"context"
	"testing"
)

func TestConnectionNegotiatesOnceAndRejectsFeatureEscalation(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, w := players[0], wires[0]
	ctx := context.Background()
	accept := []byte{1, 249, 71, 76, 79, 78, 1, 0, 5, 0, 0, 0}
	if err := s.handshakeCommand(ctx, c, accept); err == nil {
		t.Fatal("unsolicited acceptance")
	}
	for range 2 {
		if err := s.discoveryCommand(ctx, c, nil); err != nil {
			t.Fatal(err)
		}
	}
	offers := 0
	for _, p := range w.packets(t) {
		if len(p) > 1 && p[0] == 1 && p[1] >= 240 {
			offers++
			if p[1] != 248 {
				t.Fatal("per-feature offer sent", p)
			}
		}
	}
	if offers != 1 {
		t.Fatal("connection hello repeated", offers)
	}
	for _, flags := range []byte{7, 32, 255} {
		bad := append([]byte(nil), accept...)
		bad[8] = flags
		if err := s.handshakeCommand(ctx, c, bad); err == nil || c.connectionNegotiated {
			t.Fatal("unoffered feature accepted", flags)
		}
	}
	if err := s.handshakeCommand(ctx, c, accept); err != nil {
		t.Fatal(err)
	}
	if !c.connectionNegotiated || c.connectionFeatures != 5 || !c.clientBackdrops || c.combatCapabilities != 0 {
		t.Fatal("negotiated features incorrect")
	}
	w.Reset()
	if err := s.handshakeCommand(ctx, c, accept); err != nil || w.Len() != 0 {
		t.Fatal("duplicate acceptance was not idempotent", err)
	}
	changed := append([]byte(nil), accept...)
	changed[8] = 1
	if err := s.handshakeCommand(ctx, c, changed); err == nil || c.connectionFeatures != 5 {
		t.Fatal("established mode changed")
	}
	c.backdropOffered = true
	if err := s.handshakeCommand(ctx, c, []byte{1, 245, 71, 76, 66, 68, 1, 0}); err == nil {
		t.Fatal("old feature handshake bypassed connection mode")
	}
	if err := s.discoveryCommand(ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	if len(w.packets(t)) != 2 || c.connectionFeatures != 5 {
		t.Fatal("rediscovery renegotiated connection")
	}
}
