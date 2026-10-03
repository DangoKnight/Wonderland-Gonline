package server

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestMallGameNativePacketsAndNoPayment(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	before, err := json.Marshal(c.character)
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range []byte{0, 3, 4, 255} {
		if err := s.dispatch(context.Background(), c, []byte{75, 4, category}); err != nil {
			t.Fatal(err)
		}
		got := wire.packets(t)
		want := [][]byte{{57, 1, category, 0, 0, 0}, {75, 3, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, {75, 9, 30, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
		if len(got) != len(want) {
			t.Fatal(got)
		}
		for i := range want {
			if !bytes.Equal(got[i], want[i]) {
				t.Fatalf("packet %d: %v != %v", i, got[i], want[i])
			}
		}
		if c.event != nil || c.account.IM != 100 || c.account.IMBonus != 30 {
			t.Fatal("launcher changed quest ownership or cached wrong balance")
		}
		// Standalone reports have no reward callback, exactly as in AC75/AC57.
		for _, outcome := range []byte{0, 1} {
			if err := s.dispatch(context.Background(), c, []byte{57, 1, outcome}); err != nil {
				t.Fatal(err)
			}
		}
		if wire.Len() != 0 {
			t.Fatal("standalone result produced quest replies")
		}
	}
	after, err := json.Marshal(c.character)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("launcher changed character")
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := json.Marshal(chars[0])
	if err != nil || !bytes.Equal(before, persisted) {
		t.Fatal("launcher persisted rewards", err)
	}
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balances.Points != 100 || balances.Bonus != 30 {
		t.Fatal("launcher charged points", balances, err)
	}
}

func TestMallGameRefreshesAuthoritativeBalancesAndReadFailure(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	if _, err := s.Store.AdjustMallBalance(context.Background(), c.account.ID, false, -85, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.AdjustMallBalance(context.Background(), c.account.ID, true, 5, "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(context.Background(), c, []byte{75, 4, 3}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if len(got) != 3 || got[1][2] != 15 || got[2][2] != 35 || c.account.IM != 15 || c.account.IMBonus != 35 {
		t.Fatal("stale balances", got)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.dispatch(canceled, c, []byte{75, 4, 3}); err == nil || wire.Len() != 0 {
		t.Fatal("failed balance read opened game")
	}
	for _, p := range [][]byte{{75, 4}, {75, 4, 3, 0}} {
		if err := s.dispatch(context.Background(), c, p); err == nil || wire.Len() != 0 {
			t.Fatal("malformed category accepted", p)
		}
	}
}

func TestMallGameOwnershipAndPrivacy(t *testing.T) {
	for _, gate := range []string{"loading", "warp", "battle", "trade", "event", "quest minigame", "storm", "beach"} {
		t.Run(gate, func(t *testing.T) {
			s, c, wire, _ := mallFixture(t)
			event := &eventSession{}
			switch gate {
			case "loading":
				c.ready = false
			case "warp":
				c.ready = false
				c.warped = true
			case "battle":
				c.battle = &battleRun{}
			case "trade":
				c.trade = &tradeSession{}
			case "event":
				c.event = event
			case "quest minigame":
				event.onMinigame = func(byte) error { t.Fatal("arcade consumed quest result"); return nil }
				c.event = event
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			}
			err := s.dispatch(context.Background(), c, []byte{75, 4, 3})
			if gate == "loading" && err == nil {
				t.Fatal("pre-ack launch accepted")
			}
			got := wire.packets(t)
			if gate == "trade" {
				if len(got) != 1 || got[0][0] != 23 || got[0][1] != 57 {
					t.Fatal("trade guard", got)
				}
			} else if len(got) != 0 {
				t.Fatal("reserved interaction replaced", got)
			}
			if (gate == "event" || gate == "quest minigame") && c.event != event {
				t.Fatal("quest ownership changed")
			}
		})
	}
	s, players, wires := compoundFixture(t)
	if err := s.dispatch(context.Background(), players[0], []byte{75, 4, 3}); err != nil {
		t.Fatal(err)
	}
	if len(wires[0].packets(t)) != 3 || wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("private game or balances leaked")
	}
}
