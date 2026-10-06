package server

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func bankFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, p, w := friendFixture(t)
	c := p[0]
	next := c.character.Clone()
	next.Gold = 500
	next.BankGold = 100
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, wire := range w {
		wire.Reset()
	}
	return s, c, w
}

func TestBankNativeBalanceGolden(t *testing.T) {
	character := game.Character{BankGold: 0x12345678, Gold: 0x87654321}
	for _, operation := range []byte{8, 9, 10} {
		expected := []byte{45, operation, 0x78, 0x56, 0x34, 0x12, 0x21, 0x43, 0x65, 0x87}
		if packet := bankBalancePacket(&character, operation); !bytes.Equal(packet, expected) {
			t.Fatal(packet)
		}
	}
}

func TestBankDispatcherDepositWithdrawAndReload(t *testing.T) {
	s, c, w := bankFixture(t)
	tradeDo(t, s, c, []byte{45, 8})
	if packets := w[0].packets(t); len(packets) != 1 || !bytes.Equal(packets[0], []byte{45, 8, 100, 0, 0, 0, 244, 1, 0, 0}) {
		t.Fatal(packets)
	}
	tradeDo(t, s, c, []byte{45, 9, 125, 0, 0, 0})
	packets := w[0].packets(t)
	if len(packets) != 2 || !bytes.Equal(packets[0], []byte{26, 4, 119, 1, 0, 0}) || !bytes.Equal(packets[1], []byte{45, 9, 225, 0, 0, 0, 119, 1, 0, 0}) {
		t.Fatal(packets)
	}
	tradeDo(t, s, c, []byte{45, 10, 200, 0, 0, 0})
	packets = w[0].packets(t)
	if len(packets) != 2 || !bytes.Equal(packets[0], []byte{26, 4, 63, 2, 0, 0}) || !bytes.Equal(packets[1], []byte{45, 10, 25, 0, 0, 0, 63, 2, 0, 0}) {
		t.Fatal(packets)
	}
	if c.character.BankGold != 25 || c.character.Gold != 575 {
		t.Fatal(c.character)
	}
	for _, wire := range w[1:] {
		if wire.Len() != 0 {
			t.Fatal("private bank balances leaked")
		}
	}
	// Reload from the fixture DB, proving both fields were saved.
	// The existing store API lists state from SQL rather than session memory.
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(saved) != 1 || saved[0].BankGold != 25 || saved[0].Gold != 575 {
		t.Fatal(saved, err)
	}
	// Simulate a new login session loaded from persisted state.
	session := &Session{conn: &captureConn{}, character: &saved[0], account: c.account, ready: true}
	if err := s.dispatch(context.Background(), session, []byte{45, 8}); err != nil {
		t.Fatal(err)
	}
	if packets := session.conn.(*captureConn).packets(t); len(packets) != 1 || !bytes.Equal(packets[0], []byte{45, 8, 25, 0, 0, 0, 63, 2, 0, 0}) {
		t.Fatal(packets)
	}
}

func TestBankRefusesInsufficientOverflowAndCarryingLimit(t *testing.T) {
	for _, test := range []struct {
		name               string
		gold, bank, amount uint32
		operation          byte
	}{
		{"empty", 500, 100, 0, 9}, {"insufficient wallet", 500, 100, 501, 9},
		{"insufficient bank", 500, 100, 101, 10}, {"bank overflow", 1, math.MaxUint32, 1, 9},
		{"wallet cap", 999999, 100, 1, 10}, {"cross wallet cap", 999998, 100, 2, 10},
		{"large withdrawal", 0, math.MaxUint32, math.MaxUint32, 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, c, w := bankFixture(t)
			next := c.character.Clone()
			next.Gold = test.gold
			next.BankGold = test.bank
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			tradeDo(t, s, c, protocol.Builder{45, test.operation}.U32(test.amount))
			if !reflect.DeepEqual(before, *c.character) {
				t.Fatal("rejected transaction mutated state")
			}
			packets := w[0].packets(t)
			if len(packets) != 2 || packets[0][0] != 2 || packets[0][1] != 16 || packets[1][0] != 45 || packets[1][1] != 8 {
				t.Fatal("rejected transaction emitted success", packets)
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Gold != before.Gold || saved[0].BankGold != before.BankGold {
				t.Fatal(saved, err)
			}
		})
	}
}

func TestBankValidatesPacketsAndLeavesPlaceholdersUnavailable(t *testing.T) {
	s, c, w := bankFixture(t)
	before := c.character.Clone()
	for _, packet := range [][]byte{{45}, {45, 8, 0}, {45, 9}, {45, 9, 1, 2, 3}, {45, 10, 1, 2, 3}, {45, 9, 1, 2, 3, 4, 5}} {
		if err := s.dispatch(context.Background(), c, packet); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(packet, err)
		}
	}
	if w[0].Len() != 0 {
		t.Fatal("invalid packet emitted success")
	}
	for _, operation := range []byte{11, 12} {
		tradeDo(t, s, c, []byte{45, operation})
		packets := w[0].packets(t)
		if len(packets) != 1 || packets[0][0] != 2 || packets[0][1] != 16 {
			t.Fatal("placeholder claimed success", packets)
		}
	}
	if !reflect.DeepEqual(before, *c.character) {
		t.Fatal("invalid/unavailable request mutated state")
	}
}

func TestBankRespectsInteractionGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "minigame", "trade", "event", "storm", "beach"} {
		t.Run(gate, func(t *testing.T) {
			s, c, w := bankFixture(t)
			before := c.character.Clone()
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
			err := s.dispatch(context.Background(), c, []byte{45, 9, 1, 0, 0, 0})
			if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) {
				t.Fatal(err)
			}
			if gate != "loading" && err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, *c.character) {
				t.Fatal("bank bypassed gate")
			}
			for _, packet := range w[0].packets(t) {
				if packet[0] == 45 && packet[1] == 9 {
					t.Fatal("blocked mutation acknowledged")
				}
			}
		})
	}
}

func TestBankFailureBeforeSaveDoesNotPublishOrChangeState(t *testing.T) {
	s, c, w := bankFixture(t)
	before := c.character.Clone()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.dispatch(ctx, c, []byte{45, 9, 50, 0, 0, 0}); err == nil {
		t.Fatal("failed save ignored")
	}
	if !reflect.DeepEqual(before, *c.character) || w[0].Len() != 0 {
		t.Fatal("failed save mutated or published state")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Gold != before.Gold || saved[0].BankGold != before.BankGold {
		t.Fatal(saved, err)
	}
}

func TestBankReceiptFailureKeepsDurableBalances(t *testing.T) {
	s, c, _ := bankFixture(t)
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, []byte{45, 9, 50, 0, 0, 0}); err == nil {
		t.Fatal("write failure ignored")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].BankGold != 150 || saved[0].Gold != 450 || c.character.BankGold != 150 || c.character.Gold != 450 {
		t.Fatal(saved, err)
	}
}
