package server

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestBankTransferRechecksDurableFunds(t *testing.T) {
	for _, command := range []byte{29, 45} {
		t.Run(map[byte]string{29: "counter", 45: "ATM"}[command], func(t *testing.T) {
			s, c, w := bankFixture(t)
			// Deliberately leave a stale, richer session snapshot. Both paths must
			// validate the durable balance rather than recreate spent currency.
			if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error { stored.Gold = 10; stored.BankGold = 20; return nil }); err != nil {
				t.Fatal(err)
			}
			sub := byte(2)
			if command == 45 {
				sub = 9
			}
			tradeDo(t, s, c, protocol.Builder{command, sub}.U32(100))
			if c.character.Gold != 10 || c.character.BankGold != 20 {
				t.Fatal("used stale funds", c.character.Gold, c.character.BankGold)
			}
			if command == 29 {
				got := w[0].packets(t)
				want := [][]byte{{29, 3}, {29, 4, 20, 0, 0, 0}, {29, 8}}
				if !reflect.DeepEqual(got, want) {
					t.Fatal("native rejection did not unlock the form", got)
				}
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Gold != 10 || saved[0].BankGold != 20 {
				t.Fatal("rejection created funds", saved, err)
			}
		})
	}
}

func TestMoneyBankRejectsAmountsAndUnlocks(t *testing.T) {
	for _, test := range []struct {
		name               string
		gold, bank, amount uint32
		operation          byte
	}{
		{"zero", 500, 100, 0, 2}, {"wallet empty", 0, 100, 1, 2},
		{"bank full", 1, math.MaxUint32, 1, 2}, {"insufficient bank", 500, 100, 101, 1},
		{"carrying limit", 999999, 100, 1, 1}, {"oversized", 0, math.MaxUint32, math.MaxUint32, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, c, w := bankFixture(t)
			next := c.character.Clone()
			next.Gold, next.BankGold = test.gold, test.bank
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			tradeDo(t, s, c, protocol.Builder{29, test.operation}.U32(test.amount))
			got := w[0].packets(t)
			done := byte(8)
			if test.operation == 1 {
				done = 7
			}
			if len(got) != 3 || !bytes.Equal(got[0], []byte{29, 3}) || !bytes.Equal(got[2], []byte{29, done}) {
				t.Fatal("rejection locked the native window", got)
			}
			if c.character.Gold != test.gold || c.character.BankGold != test.bank {
				t.Fatal("rejection changed balances")
			}
		})
	}
}

func TestMoneyBankRespectsAllInteractionGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "minigame", "trade", "event", "storm", "beach"} {
		t.Run(gate, func(t *testing.T) {
			s, c, w := bankFixture(t)
			switch gate {
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "minigame":
				c.event = &eventSession{onMinigame: func(byte) error { t.Fatal("bank consumed minigame"); return nil }}
			case "trade":
				c.trade = &tradeSession{}
			case "event":
				c.event = &eventSession{}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			}
			before := c.character.Clone()
			err := s.dispatch(context.Background(), c, []byte{29, 2, 50, 0, 0, 0})
			if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) || gate != "loading" && err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, *c.character) {
				t.Fatal("bank bypassed an interaction gate", gate)
			}
			for _, p := range w[0].packets(t) {
				if len(p) > 0 && p[0] == 29 {
					t.Fatal("blocked bank command published a receipt", gate, p)
				}
			}
		})
	}
}

func TestBankAuthoredServiceCompletesWithoutResumeAck(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, w := players[0], wires[0]
	ev := &assets.Event{ClickID: 65000, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, world.ActionServiceOrTeleport, world.ServiceBank, 0, 0, 0)}}}}
	if err := s.startEvent(context.Background(), c, 77, ev, 0, false); err != nil {
		t.Fatal(err)
	}
	packets := w.packets(t)
	resume, bank := -1, -1
	for i, p := range packets {
		if bytes.Equal(p, []byte{20, 8}) {
			resume = i
		}
		if bytes.Equal(p, []byte{29, 5}) {
			bank = i
		}
	}
	if resume < 0 || bank <= resume || c.event != nil {
		t.Fatal("bank did not open after completing the authored service", packets)
	}
	w.Reset()
	for range 3 {
		if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
			t.Fatal(err)
		}
	}
	if w.Len() != 0 {
		t.Fatal("completed service responded to idle ACKs", w.packets(t))
	}
}

func TestBankTransferPreservesBufferedRecoveryAndWalking(t *testing.T) {
	s, c, _ := bankFixture(t)
	next := c.character.Clone()
	next.HP = 10
	next.SP = 5
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	c.character.X += 20
	c.character.HP += 3
	c.character.SP += 2
	tradeDo(t, s, c, []byte{29, 2, 50, 0, 0, 0})
	if c.character.X != next.X+20 || c.character.HP != 13 || c.character.SP != 7 || c.character.Gold != 450 || c.character.BankGold != 150 {
		t.Fatal("bank replaced pending state", c.character)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].X != next.X || saved[0].HP != 10 || saved[0].SP != 5 || saved[0].Gold != 450 || saved[0].BankGold != 150 {
		t.Fatal("bank checkpointed unrelated state", saved, err)
	}
	s.autosaveCharacters(context.Background())
	saved, err = s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].X != next.X+20 || saved[0].HP != 13 || saved[0].SP != 7 || saved[0].Gold != 450 || saved[0].BankGold != 150 {
		t.Fatal("checkpoint lost pending state or replaced currency", saved, err)
	}
}

func TestMoneyBankMalformedAndFailureBoundaries(t *testing.T) {
	s, c, w := bankFixture(t)
	before := c.character.Clone()
	for _, p := range [][]byte{{29}, {29, 1}, {29, 2, 1}, {29, 1, 0, 0, 0}, {29, 2, 1, 0, 0, 0, 0}} {
		if err := s.dispatch(context.Background(), c, p); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal("accepted malformed currency request", p, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.dispatch(ctx, c, []byte{29, 2, 50, 0, 0, 0}); err == nil {
		t.Fatal("cancelled transaction succeeded")
	}
	if w[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("failed operation changed state or published a receipt")
	}
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, []byte{29, 2, 50, 0, 0, 0}); err == nil {
		t.Fatal("receipt failure ignored")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Gold != 450 || saved[0].BankGold != 150 {
		t.Fatal("receipt failure refunded a committed transfer", saved, err)
	}
}

func TestBankServiceCompletionAndCancellationOwnership(t *testing.T) {
	for _, kind := range []string{"cancel", "replacement", "character"} {
		t.Run(kind, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c, w := players[0], wires[0]
			es := &eventSession{mapID: c.character.Map}
			c.event = es
			if err := s.openService(c, 6); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "cancel":
				if err := s.cancelInteraction(c); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				s.endEvent(c)
				c.event = &eventSession{mapID: c.character.Map}
			case "character":
				c.character = &game.Character{ID: c.character.ID + 1, Map: c.character.Map}
			}
			if err := s.finishEvent(c, es); err != nil {
				t.Fatal(err)
			}
			for _, p := range w.packets(t) {
				if bytes.Equal(p, []byte{29, 5}) {
					t.Fatal("opened a cancelled or foreign bank service", kind)
				}
			}
		})
	}
}

func TestBankIdleCompletionAndLostCharacterDoNotPublish(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, w := players[0], wires[0]
	if err := s.finishEvent(c, nil); err != nil {
		t.Fatal(err)
	}
	es := &eventSession{mapID: c.character.Map}
	c.event = es
	if err := s.openService(c, 6); err != nil {
		t.Fatal(err)
	}
	c.character = nil
	if err := s.finishEvent(c, es); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || es.bankOwner != 0 || w.Len() != 0 {
		t.Fatal("lost-character completion retained or published a bank service")
	}
}
