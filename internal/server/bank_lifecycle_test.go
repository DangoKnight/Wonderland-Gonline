package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

func TestBankLateAcknowledgementDoesNotRecloseWindow(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	c.event = &eventSession{mapID: c.character.Map}
	if err := s.openService(c, world.ServiceBank); err != nil {
		t.Fatal(err)
	}
	if err := s.finishEvent(c, c.event); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{29, 5}) {
		t.Fatal("bank did not open")
	}
	wire.Reset()
	// The resume's matching client acknowledgement may arrive after the
	// 400ms window on a slow connection. It still belongs to the old close.
	time.Sleep(450 * time.Millisecond)
	if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	for _, packet := range wire.packets(t) {
		if bytes.Equal(packet, []byte{20, 8}) {
			t.Fatal("late acknowledgement closes the bank and restarts the resume/ack loop")
		}
	}
}

func TestBankPendingOpenDoesNotSurviveTeleport(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	s.Assets.Maps[20000] = assets.Map{ID: 20000}
	s.World = world.New(s.Assets)
	s.world[c.info.ID] = c
	c.ready = true
	c.event = &eventSession{mapID: c.character.Map}
	if err := s.openService(c, world.ServiceBank); err != nil {
		t.Fatal(err)
	}
	if err := s.commandTeleport(context.Background(), c, world.Destination{Map: 20000, X: 500, Y: 600}); err != nil {
		t.Fatal(err)
	}
	if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	playersWire := c.conn.(*captureConn)
	playersWire.Reset()
	if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if contains(playersWire.packets(t), []byte{29, 5}) {
		t.Fatal("acknowledging on the destination map opens the bank requested on the previous map")
	}
}
