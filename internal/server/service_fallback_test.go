package server

import (
	"context"
	"testing"
	"wonderland-go/internal/game"
	"wonderland-go/internal/world"
)

func TestFallbackServiceClassification(t *testing.T) {
	for _, tc := range []struct {
		n    world.NPC
		want byte
	}{
		{world.NPC{Template: 14134}, 4}, {world.NPC{Name: "Stock Keeper"}, 9},
		{world.NPC{Name: "Witch Doctor Hotel"}, 6}, {world.NPC{Name: "PET KEEPER"}, 5},
		{world.NPC{Template: 13006}, 1}, {world.NPC{Template: 13005}, 2},
		{world.NPC{Name: "Villager"}, 0},
	} {
		if got := serviceKind(tc.n); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestDoctorHotelChoiceWithoutDatabase(t *testing.T) {
	s := &Server{}
	wire := &captureConn{}
	c := &Session{conn: wire, character: &game.Character{Map: 10017}}
	handled, err := s.fallbackService(context.Background(), c, world.NPC{ClickID: 22, Template: 14151})
	if !handled || err != nil || !contains(wire.packets(t), serviceQuestion(22, 3, 1)) {
		t.Fatal("doctor menu", handled, err)
	}
	if err := s.eventCommand(context.Background(), c, []byte{20, 9, 32}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if !contains(got, []byte{31, 7}) || !contains(got, []byte{6, 2, 0}) || c.event != nil {
		t.Fatal("hotel choice", got)
	}
	if err := s.eventCommand(context.Background(), c, []byte{20, 9, 32}); err != nil {
		t.Fatal(err)
	}
	if len(wire.packets(t)) != 0 {
		t.Fatal("late choice reopened hotel")
	}
}
