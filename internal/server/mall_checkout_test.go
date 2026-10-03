package server

import (
	"bytes"
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

func TestMallCheckoutNativeGolden(t *testing.T) {
	packets := mallCheckoutPackets(store.MallBalances{Points: 0x12345678, Bonus: 0x11223344})
	expected := [][]byte{
		{75, 3, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0, 0, 0, 0},
		{75, 9, 0x44, 0x33, 0x22, 0x11, 0, 0, 0, 0, 0, 0, 0},
		{35, 4, 0x78, 0x56, 0x34, 0x12, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	if !reflect.DeepEqual(packets, expected) {
		t.Fatal(packets)
	}
}

func TestMallCheckoutModesUseCurrentSQLBalancesWithoutCatalog(t *testing.T) {
	for _, mode := range []byte{0, 1} {
		t.Run(string(rune('0'+mode)), func(t *testing.T) {
			s, c, wire, _ := mallFixture(t)
			ctx := context.Background()
			before := c.character.Clone()
			c.account.IM, c.account.IMBonus = 900, 800
			if _, err := s.Store.AdjustMallBalance(ctx, c.account.ID, false, 17, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Store.AdjustMallBalance(ctx, c.account.ID, true, 9, "test"); err != nil {
				t.Fatal(err)
			}
			tradeDo(t, s, c, []byte{34, 1, mode})
			packets := wire.packets(t)
			expected := [][]byte{{75, 3, 117, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, {75, 9, 39, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, {35, 4, 117, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
			if !reflect.DeepEqual(packets, expected) {
				t.Fatal("checkout cleared cart or returned stale/mode-dependent points", packets)
			}
			if c.account.IM != 117 || c.account.IMBonus != 39 || !reflect.DeepEqual(before, *c.character) {
				t.Fatal("checkout mutated gameplay state or cached wrong balances")
			}
			balances, err := s.Store.MallBalances(ctx, c.account.ID)
			if err != nil || balances.Points != 117 || balances.Bonus != 39 {
				t.Fatal("balance query spent points", balances, err)
			}
		})
	}
}

func TestMallCheckoutThenCartPurchase(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	tradeDo(t, s, c, []byte{34, 1, 0})
	wire.Reset()
	tradeDo(t, s, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 2, order: 12}))
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balances.Points != 80 || balances.Bonus != 30 || c.character.Bag[0].ID != 32176 || c.character.Bag[0].Count != 10 {
		t.Fatal(balances, c.character.Bag, err)
	}
	if !contains(wire.packets(t), []byte{75, 4, 176, 125, 4, 2, 12, 0, 1}) {
		t.Fatal("checkout failed to continue purchase")
	}
}

func TestMallCheckoutValidationAndReadFailure(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	before := c.character.Clone()
	account := c.account
	for _, packet := range [][]byte{{34}, {34, 1}, {34, 1, 2}, {34, 1, 0, 0}} {
		if err := s.dispatch(context.Background(), c, packet); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(packet, err)
		}
	}
	if err := s.dispatch(context.Background(), c, []byte{34, 2, 0}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	if wire.Len() != 0 || !reflect.DeepEqual(before, *c.character) || c.account != account {
		t.Fatal("invalid query changed state")
	}
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{34, 1, 0}); err == nil {
		t.Fatal("failed balance read ignored")
	}
	if wire.Len() != 0 || c.account != account {
		t.Fatal("failed read emitted checkout success or changed cache")
	}
}

func TestMallCheckoutWorldGatesAndReadOnlyTrade(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "minigame", "trade"} {
		t.Run(gate, func(t *testing.T) {
			s, c, wire, _ := mallFixture(t)
			switch gate {
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "minigame":
				c.event = &eventSession{onMinigame: func(byte) error { t.Fatal("checkout consumed minigame"); return nil }}
			case "trade":
				c.trade = &tradeSession{}
			}
			err := s.dispatch(context.Background(), c, []byte{34, 1, 0})
			if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) {
				t.Fatal(err)
			}
			if gate != "loading" && err != nil {
				t.Fatal(err)
			}
			if gate == "trade" {
				if packets := wire.packets(t); len(packets) != 3 || packets[2][0] != 35 || packets[2][1] != 4 || c.trade == nil {
					t.Fatal("read-only checkout cancelled trade", packets)
				}
			} else if wire.Len() != 0 {
				t.Fatal("checkout bypassed world interaction gate")
			}
		})
	}
}

func TestMallCheckoutWireRangeMatchesBalanceReplies(t *testing.T) {
	for _, test := range []struct {
		value    int64
		expected []byte
	}{
		{-1, []byte{0, 0, 0, 0}}, {0, []byte{0, 0, 0, 0}},
		{math.MaxInt32, []byte{255, 255, 255, 127}}, {math.MaxInt64, []byte{255, 255, 255, 127}},
	} {
		packets := mallCheckoutPackets(store.MallBalances{Points: test.value, Bonus: test.value})
		for _, packet := range packets {
			if !bytes.Equal(packet[2:6], test.expected) {
				t.Fatal("inconsistent wire point range", packet)
			}
		}
	}
}

func TestMallCheckoutReceiptFailureDoesNotSpendOrDeliver(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	before := c.character.Clone()
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, []byte{34, 1, 0}); err == nil {
		t.Fatal("failed receipt ignored")
	}
	balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balances.Points != 100 || balances.Bonus != 30 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal(balances, err)
	}
}
