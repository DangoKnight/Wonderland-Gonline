package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
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

func TestMerchantFallbackSeparatesBuyAndSell(t *testing.T) {
	for _, buy := range []bool{false, true} {
		name := "sell"
		if buy {
			name = "buy"
		}
		t.Run(name, func(t *testing.T) {
			s, players, wires := tradeFixture(t)
			c := players[0]
			handled, err := s.fallbackService(context.Background(), c, world.NPC{ClickID: 22, Template: 13007})
			if !handled || err != nil {
				t.Fatal(handled, err)
			}
			if err := c.event.onChoice(30); err != nil {
				t.Fatal(err)
			}
			wires[0].Reset()
			choice := byte(31)
			if buy {
				choice = 30
			}
			if err := c.event.onChoice(choice); err != nil {
				t.Fatal(err)
			}
			packets := wires[0].packets(t)
			hasBuy, hasSell := false, false
			for _, p := range packets {
				if len(p) >= 2 && p[0] == 35 && p[1] == 12 {
					hasBuy = true
				}
				if len(p) == 2 && p[0] == 27 && p[1] == 3 {
					hasSell = true
				}
			}
			if hasBuy != buy || hasSell == buy || c.event != nil || !buy && len(c.view.ShopItems) != 0 {
				t.Fatal("merchant choice opened the wrong service", packets)
			}
		})
	}
}
