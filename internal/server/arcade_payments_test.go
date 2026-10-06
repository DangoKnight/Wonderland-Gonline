package server

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func paidArcadeFixture(t *testing.T, kind byte) (*Server, *Session, *captureConn) {
	t.Helper()
	s, c, wire, _ := mallFixture(t)
	id := assets.ArcadePrizeItem(kind, 1)
	s.Assets.Items[id] = game.ItemDefinition{ID: id, Type: 23}
	s.Assets.Arcades = []assets.ArcadeGame{{Kind: kind, Enabled: true, PointCost: 5, TokenCount: 4, CooldownMilliseconds: 3000, Rewards: []assets.ArcadeReward{{Index: 1, ItemID: id, Quantity: 1, Weight: 1, Reels: [3]byte{1, 1, 1}}}}}
	if err := s.dispatch(context.Background(), c, []byte{75, 4, kind}); err != nil {
		t.Fatal(err)
	}
	wire.packets(t)
	return s, c, wire
}
func arcadeSaveBag(t *testing.T, s *Server, c *Session) {
	t.Helper()
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(next *game.Character) error { next.Bag = c.character.Bag; return nil }); err != nil {
		t.Fatal(err)
	}
}
func arcadeRequest(kind byte) []byte {
	if kind == 10 {
		return []byte{71, 10, 0}
	}
	if kind == 8 || kind == 19 {
		return []byte{71, kind, 1, 0}
	}
	return []byte{71, kind, 1}
}
func TestArcadeNativePaidRepliesAndDurability(t *testing.T) {
	for _, kind := range []byte{6, 8, 10, 19, 22} {
		t.Run(string(rune(kind+'A')), func(t *testing.T) {
			s, c, wire := paidArcadeFixture(t, kind)
			if err := s.dispatch(context.Background(), c, arcadeRequest(kind)); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			want := []byte{71, kind, 1, 1, 1}
			if kind == 8 || kind == 10 || kind == 19 {
				want = []byte{71, kind, 1, 1, 1, 1, 1, 1}
			}
			if len(got) != 4 || !bytes.Equal(got[len(got)-1], want) || got[0][0] != 23 || got[0][1] != 5 || got[1][2] != 95 || got[2][2] != 30 {
				t.Fatal(got, want)
			}
			balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
			chars, loadErr := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || loadErr != nil || balances.Points != 95 || balances.Bonus != 30 || chars[0].Bag != c.character.Bag {
				t.Fatal("not committed", balances, err, loadErr)
			}
			count := 0
			for _, item := range chars[0].Bag {
				if item.ID == assets.ArcadePrizeItem(kind, 1) {
					count += int(item.Count)
				}
			}
			if count != 1 {
				t.Fatal("incorrect grant", count)
			}
			// A duplicate cannot debit again, including after close/reopen.
			if err := s.dispatch(context.Background(), c, arcadeRequest(kind)); err != nil {
				t.Fatal(err)
			}
			packets := wire.packets(t)
			if packets[len(packets)-1][2] != 2 {
				t.Fatal("duplicate accepted", packets)
			}
			if err := s.dispatch(context.Background(), c, []byte{57, 1, 0}); err != nil {
				t.Fatal(err)
			}
			if c.arcade != nil || !bytes.Equal(wire.packets(t)[0], []byte{57, 2}) {
				t.Fatal("machine not closed")
			}
			if err := s.dispatch(context.Background(), c, []byte{75, 4, kind}); err != nil {
				t.Fatal(err)
			}
			wire.packets(t)
			if err := s.dispatch(context.Background(), c, arcadeRequest(kind)); err != nil {
				t.Fatal(err)
			}
			wire.packets(t)
			balances, _ = s.Store.MallBalances(context.Background(), c.account.ID)
			if balances.Points != 95 {
				t.Fatal("reopen bypassed throttle")
			}
			c.arcadeNextPurchaseAt = time.Time{}
			if err := s.dispatch(context.Background(), c, arcadeRequest(kind)); err != nil {
				t.Fatal(err)
			}
			wire.packets(t)
			balances, _ = s.Store.MallBalances(context.Background(), c.account.ID)
			if balances.Points != 90 {
				t.Fatal("intentional replay rejected")
			}
		})
	}
}
func TestArcadeTokenAndVoucherPayments(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    byte
		request []byte
		item    uint16
		count   byte
	}{
		{"fun tokens", 6, []byte{71, 6, 2}, 34007, 4},
		{"play tokens", 22, []byte{71, 22, 3}, 34008, 4},
		{"slot tokens", 8, []byte{71, 8, 2, 0}, 34007, 4},
		{"slot voucher", 19, []byte{71, 19, 1, 1}, 34360, 1},
		{"slot2 voucher", 10, []byte{71, 10, 1}, 34360, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, wire := paidArcadeFixture(t, tc.kind)
			c.character.Bag = game.Inventory{}
			// Token consumption must span stacks. Vouchers consume exactly the selected slot.
			c.character.Bag[0] = game.Item{ID: tc.item, Count: 1}
			if tc.count > 1 {
				c.character.Bag[1] = game.Item{ID: tc.item, Count: tc.count - 1}
			}
			arcadeSaveBag(t, s, c)
			if err := s.dispatch(context.Background(), c, tc.request); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			if !bytes.Equal(got[0], []byte{23, 9, 1, 1}) || got[len(got)-1][2] != 1 {
				t.Fatal(got)
			}
			for _, item := range c.character.Bag {
				if item.ID == tc.item {
					t.Fatal("payment not removed")
				}
			}
			balances, _ := s.Store.MallBalances(context.Background(), c.account.ID)
			if balances.Points != 100 || balances.Bonus != 30 {
				t.Fatal("token/voucher charged points", balances)
			}
		})
	}
}
func TestArcadeRejectionsRollBackPaymentAndDelivery(t *testing.T) {
	for _, reason := range []string{"funds", "full", "token shortfall", "token full", "locked voucher", "wrong voucher", "disabled", "no ownership", "wrong kind", "wrong map", "event replaced"} {
		t.Run(reason, func(t *testing.T) {
			s, c, wire := paidArcadeFixture(t, 8)
			request := []byte{71, 8, 1, 0}
			c.character.Bag = game.Inventory{}
			switch reason {
			case "funds":
				s.Assets.Arcades[0].PointCost = 101
			case "full":
				for i := range c.character.Bag {
					c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
				}
			case "token shortfall":
				c.character.Bag[0] = game.Item{ID: 34007, Count: 3}
				request[2] = 2
			case "token full":
				for i := range c.character.Bag {
					c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
				}
				c.character.Bag[0] = game.Item{ID: 34007, Count: 5}
				request[2] = 2
			case "locked voucher":
				c.character.Bag[0] = game.Item{ID: 34360, Count: 1, Locked: true}
				request[3] = 1
			case "wrong voucher":
				c.character.Bag[0] = game.Item{ID: 32176, Count: 1}
				request[3] = 1
			case "disabled":
				s.Assets.Arcades[0].Enabled = false
			case "no ownership":
				c.arcade = nil
			case "wrong kind":
				c.arcade.kind = 19
			case "wrong map":
				c.arcade.mapID++
			case "event replaced":
				c.event = &eventSession{}
			}
			arcadeSaveBag(t, s, c)
			before := c.character.Bag
			if err := s.dispatch(context.Background(), c, request); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			if len(got) != 2 || !bytes.Equal(got[1], []byte{71, 8, 2}) {
				t.Fatal(got)
			}
			balances, err := s.Store.MallBalances(context.Background(), c.account.ID)
			chars, loadErr := s.Store.Characters(context.Background(), c.account.ID)
			expected := before
			for i := range expected {
				expected[i].Locked = false
			}
			if err != nil || loadErr != nil || balances.Points != 100 || chars[0].Bag != expected || c.character.Bag != before || !c.arcadeNextPurchaseAt.IsZero() {
				t.Fatal("rejection mutated state", balances, err, loadErr)
			}
		})
	}
}
func TestArcadeMalformedAndFailedTransaction(t *testing.T) {
	s, c, wire := paidArcadeFixture(t, 8)
	for _, p := range [][]byte{{71}, {71, 6}, {71, 8, 1}, {71, 8, 0, 0}, {71, 8, 4, 0}, {71, 8, 1, 51}, {71, 8, 2, 1}, {71, 10, 0, 0}, {71, 6, 1, 0}} {
		if err := s.dispatch(context.Background(), c, p); !errors.Is(err, protocol.ErrMalformed) || wire.Len() != 0 {
			t.Fatal(p, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.dispatch(ctx, c, []byte{71, 8, 1, 0}); err == nil || wire.Len() != 0 || !c.arcadeNextPurchaseAt.IsZero() {
		t.Fatal("failed transaction emitted success")
	}
	balances, _ := s.Store.MallBalances(context.Background(), c.account.ID)
	if balances.Points != 100 {
		t.Fatal("failed transaction charged points")
	}
}

func TestArcadeSocketFailurePreservesCommittedPlay(t *testing.T) {
	s, c, wire := paidArcadeFixture(t, 6)
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, []byte{71, 6, 1}); err == nil {
		t.Fatal("socket error hidden")
	}
	balances, _ := s.Store.MallBalances(context.Background(), c.account.ID)
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || balances.Points != 95 || chars[0].Bag != c.character.Bag || c.arcadeNextPurchaseAt.IsZero() {
		t.Fatal("committed play lost", err)
	}
	c.conn = wire
	if err := s.dispatch(context.Background(), c, []byte{71, 6, 1}); err != nil {
		t.Fatal(err)
	}
	balances, _ = s.Store.MallBalances(context.Background(), c.account.ID)
	if balances.Points != 95 {
		t.Fatal("retry charged twice")
	}
}

func TestArcadeEventOwnershipAndResultAreSeparate(t *testing.T) {
	s, c, wire := paidArcadeFixture(t, 6)
	called := 0
	es := &eventSession{mapID: c.character.Map, onMinigame: func(result byte) error { called++; return nil }}
	c.event = es
	c.arcade.event = es
	if err := s.dispatch(context.Background(), c, []byte{71, 6, 1}); err != nil {
		t.Fatal(err)
	}
	wire.packets(t)
	if called != 0 || es.onMinigame == nil {
		t.Fatal("purchase consumed event result")
	}
	if err := s.dispatch(context.Background(), c, []byte{57, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if called != 1 || c.arcade != nil || es.onMinigame != nil || !bytes.Equal(wire.packets(t)[0], []byte{57, 2}) {
		t.Fatal("event result not consumed once")
	}
	if err := s.dispatch(context.Background(), c, []byte{57, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if called != 1 || wire.Len() != 0 {
		t.Fatal("duplicate result replayed")
	}
	c.arcade = &arcadeSession{kind: 6, mapID: c.character.Map}
	s.endEvent(c)
	if c.arcade != nil {
		t.Fatal("world lifecycle kept stale machine")
	}
}

func TestArcadeCannotForcePrizesWithInventorySpace(t *testing.T) {
	s, c, wire := paidArcadeFixture(t, 8)
	other := assets.ArcadePrizeItem(8, 2)
	s.Assets.Items[other] = game.ItemDefinition{ID: other, Type: 23}
	s.Assets.Arcades[0].Rewards = append(s.Assets.Arcades[0].Rewards, assets.ArcadeReward{Index: 2, ItemID: other, Quantity: 1, Weight: 1, Reels: [3]byte{3, 3, 3}})
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	c.character.Bag[0] = game.Item{ID: assets.ArcadePrizeItem(8, 1), Count: 49}
	arcadeSaveBag(t, s, c)
	if err := s.dispatch(context.Background(), c, []byte{71, 8, 1, 0}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if len(got) != 2 || got[1][2] != 2 {
		t.Fatal("selective inventory allowed a play", got)
	}
	balances, _ := s.Store.MallBalances(context.Background(), c.account.ID)
	if balances.Points != 100 {
		t.Fatal("rejected play charged")
	}
}
