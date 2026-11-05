package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
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

// TestArcadeFruitMachines: Mario 1 pays with its kind alone, Mario 2 and 3
// with a payment byte; three matching reels come back as reply 3, other
// lines as reply 4 with the lamp, and a full bag as reply 5.
func TestArcadeFruitMachines(t *testing.T) {
	for _, tc := range []struct {
		kind    byte
		request []byte
		reels   [3]byte
		want    []byte
	}{
		{11, []byte{71, 11}, [3]byte{4, 4, 4}, []byte{71, 11, 3, 1, 4, 4, 4, 1}},
		{12, []byte{71, 12, 1}, [3]byte{2, 8, 5}, []byte{71, 12, 4, 1, 2, 8, 5, 1, 1}},
		{14, []byte{71, 14, 1}, [3]byte{8, 8, 8}, []byte{71, 14, 3, 1, 8, 8, 8, 1}},
	} {
		s, c, wire := paidArcadeFixture(t, tc.kind)
		s.Assets.Arcades[0].Rewards[0].Reels = tc.reels
		if err := s.dispatch(context.Background(), c, tc.request); err != nil {
			t.Fatal(err)
		}
		got := wire.packets(t)
		if len(got) == 0 || !bytes.Equal(got[len(got)-1], tc.want) {
			t.Fatalf("kind %d: %x, want %x", tc.kind, got, tc.want)
		}
	}
	for _, bad := range [][]byte{{71, 11, 1}, {71, 12}, {71, 14, 1, 0}} {
		if _, err := parseArcadePayment(bad); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatalf("%x: %v", bad, err)
		}
	}
	// Mario 1 takes no tokens.
	s, c, wire := paidArcadeFixture(t, 11)
	s.Assets.Arcades[0].TokenCount = 0
	if err := s.dispatch(context.Background(), c, []byte{71, 11}); err != nil {
		t.Fatal(err)
	}
	wire.packets(t)
	// A full bag answers reply 5 and takes nothing.
	s, c, wire = paidArcadeFixture(t, 12)
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 10002, Count: 1}
	}
	s.Assets.Items[10002] = game.ItemDefinition{ID: 10002, Type: 4}
	arcadeSaveBag(t, s, c)
	if err := s.dispatch(context.Background(), c, []byte{71, 12, 1}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	balances, _ := s.Store.MallBalances(context.Background(), c.account.ID)
	if len(got) != 1 || !bytes.Equal(got[0], []byte{71, 12, 5}) || balances.Points != 100 {
		t.Fatalf("full bag: %x, points %d", got, balances.Points)
	}
}

// TestArcadeBalanceDuringMinigame: the fruit machines' AC34:1 query is
// answered while an event minigame holds the player.
func TestArcadeBalanceDuringMinigame(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	c.event = &eventSession{onMinigame: func(byte) error { return nil }}
	if err := s.dispatch(context.Background(), c, []byte{34, 1, 0}); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if len(got) == 0 || got[len(got)-1][0] != 35 || got[len(got)-1][1] != 4 {
		t.Fatalf("no 35/4: %x", got)
	}
}

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

func minigameFixture(t *testing.T, gameType, seed uint16) (*Server, *Session, *captureConn, *assets.Event) {
	s, c, wire := eventFixture(t)
	ev := &assets.Event{ClickID: 10, Branches: []assets.Branch{
		{Index: 1, Operations: []assets.Operation{evOp(1, 9, gameType, seed, 0, 0), evOp(2, 1, 1, 2, 0, 999)}},
		{Index: 2, Condition: evCond(8, gameType, 0, 0, 5, 1), Operations: []assets.Operation{evOp(1, 2, 5, 0, 20363, 0), evOp(2, 1, 1, 1, 32176, 10)}},
		{Index: 3, Condition: evCond(8, gameType, 0, 0, 5, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 20364, 0)}},
	}}
	if err := s.startEvent(context.Background(), c, 5, ev, 0, false); err != nil {
		t.Fatal(err)
	}
	return s, c, wire, ev
}

func TestMinigameNativeStartResultsAndReplay(t *testing.T) {
	for _, result := range []byte{0, 1} {
		s, c, wire, _ := minigameFixture(t, 3, 0)
		packets := wire.packets(t)
		if len(packets) != 3 || !contains(packets, []byte{57, 1, 3, 0xf8, 0x2a, 1}) || !contains(packets, []byte{20, 9}) {
			t.Fatal("native minigame start", packets)
		}
		original := c.character.Bag
		es := c.event
		stale := es.onMinigame
		tradeDo(t, s, c, []byte{20, 6})
		if wire.Len() != 0 || c.event != es || es.onMinigame == nil {
			t.Fatal("ACK advanced minigame")
		}
		for _, p := range [][]byte{{57, 1}, {57, 1, 2}, {57, 1, 1, 0}} {
			tradeDo(t, s, c, p)
		}
		if wire.Len() != 0 || es.onMinigame == nil {
			t.Fatal("malformed result consumed callback")
		}
		tradeDo(t, s, c, []byte{57, 1, result})
		p := wire.packets(t)
		talk := uint16(20364)
		branch := byte(3)
		if result == 1 {
			talk = 20363
			branch = 2
		}
		if len(p) != 3 || !contains(p, []byte{57, 2}) || !contains(p, serverwire.EventFrame(1, 3, 5, 1, 0, talk, 1, branch)) {
			t.Fatal("outcome packets", p)
		}
		if c.character.Bag != original || c.character.Gold != 0 {
			t.Fatal("reward before outcome dialogue")
		}
		tradeDo(t, s, c, []byte{57, 1, result})
		if wire.Len() != 0 {
			t.Fatal("result replayed during dialogue")
		}
		// The client acknowledges its own result (FUN_003bdfa4 → 20/6)
		// before the outcome dialogue is answered.
		tradeDo(t, s, c, []byte{20, 6})
		if wire.Len() != 0 || c.event == nil {
			t.Fatal("the result's acknowledgment closed the outcome dialogue")
		}
		tradeDo(t, s, c, []byte{20, 6})
		if c.event != nil || c.character.Gold != 0 {
			t.Fatal("minigame fell through origin branch")
		}
		saved := c.character.Bag
		if err := stale(1); err != nil || c.character.Bag != saved {
			t.Fatal("stale callback rewarded", err)
		}
		tradeDo(t, s, c, []byte{57, 1, 1})
		if c.character.Bag != saved {
			t.Fatal("late result rewarded")
		}
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || chars[0].Bag != saved {
			t.Fatal("outcome not durable", err)
		}
		if result == 1 && saved == original {
			t.Fatal("win did not give reward")
		}
		if result == 0 && saved != original {
			t.Fatal("loss gave reward")
		}
	}
}

func TestMinigameAuthoredPigVariantGolden(t *testing.T) {
	s, c, wire := eventFixture(t)
	es := &eventSession{mapID: c.character.Map, ev: &assets.Event{}}
	c.event = es
	// Native Kelan event 17: opcode 9, kind 5, parameter 11000, variant 2.
	// Keep the source bytes and expected packet independent of the encoder.
	var source assets.Operation
	copy(source.Data[:], []byte{9, 5, 0, 248, 42, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if err := s.startMinigame(context.Background(), c, es, world.DecodeOp(source)); err != nil {
		t.Fatal(err)
	}
	packets := wire.packets(t)
	if !contains(packets, []byte{57, 1, 5, 248, 42, 2}) || !contains(packets, []byte{20, 9}) {
		t.Fatal("authored pig variant was replaced by the sheep default", packets)
	}
}

func TestMinigameCancellationAndLock(t *testing.T) {
	for _, action := range []string{"cancel", "warp", "disconnect"} {
		s, c, wire, _ := minigameFixture(t, 4, 0x2710)
		if !contains(wire.packets(t), []byte{57, 1, 4, 0x10, 0x27, 1}) {
			t.Fatal("authored seed ignored")
		}
		es := c.event
		callback := es.onMinigame
		bag := c.character.Bag
		x := c.character.X
		tradeDo(t, s, c, []byte{23, 124, 1, 1, 0})
		tradeDo(t, s, c, []byte{6, 1, 0, 1, 0, 1, 0})
		if c.character.Bag != bag || c.character.X != x || wire.Len() != 0 {
			t.Fatal("game allowed inventory/movement mutation")
		}
		switch action {
		case "cancel":
			tradeDo(t, s, c, []byte{20, 9, 40})
		case "warp":
			s.Assets.Maps[20000] = assets.Map{ID: 20000}
			s.World = world.New(s.Assets)
			if err := s.commandTeleport(context.Background(), c, world.Destination{Map: 20000, X: 1, Y: 1}); err != nil {
				t.Fatal(err)
			}
		case "disconnect":
			s.leaveWorld(c)
		}
		if c.event != nil {
			t.Fatal("cancel retained game", action)
		}
		if err := callback(1); err != nil || c.character.Bag != bag {
			t.Fatal("cancelled callback rewarded", action, err)
		}
	}
}

func TestMinigameMissingOutcomeAndFailedRewardSave(t *testing.T) {
	s, c, wire, ev := minigameFixture(t, 1, 0)
	wire.Reset()
	ev.Branches = ev.Branches[:1]
	tradeDo(t, s, c, []byte{57, 1, 1})
	if c.event != nil || !contains(wire.packets(t), []byte{20, 8}) {
		t.Fatal("missing outcome retained interaction")
	}
	s, c, wire, _ = minigameFixture(t, 1, 0)
	wire.Reset()
	bag := c.character.Bag
	tradeDo(t, s, c, []byte{57, 1, 1})
	tradeDo(t, s, c, []byte{20, 6}) // the result's own acknowledgment
	wire.Reset()
	s.Store.Close()
	if err := s.dispatch(context.Background(), c, []byte{20, 6}); err == nil {
		t.Fatal("failed reward save ignored")
	}
	if c.character.Bag != bag || contains(wire.packets(t), []byte{20, 10}) {
		t.Fatal("failed save published reward")
	}
}

func TestNativeRabbitMinigameRewards(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range catalog.Maps[12000].NPCs {
		for _, event := range actor.Events {
			if event == 41 {
				t.Logf("rabbit actor=%d x=%d y=%d", actor.ClickID, actor.X, actor.Y)
			}
		}
	}
	for _, result := range []byte{0, 1} {
		s, c, wire := eventFixture(t)
		s.Assets = catalog
		s.World = world.New(catalog)
		next := c.character.Clone()
		next.Map = 12000
		next.Bag = game.Inventory{}
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		ev, ok := s.World.Event(12000, 41)
		if !ok {
			t.Fatal("native rabbit event missing")
		}
		branch := s.World.FindBranch(c.character, c.view, 12000, ev, 0, 0, 0, -1)
		if branch < 0 {
			t.Fatal("rabbit entry missing")
		}
		if err := s.startEvent(context.Background(), c, 41, ev, branch, false); err != nil {
			t.Fatal(err)
		}
		tradeDo(t, s, c, []byte{20, 9, 30})
		if c.event == nil || c.event.onMinigame == nil {
			t.Fatal("rabbit failed to launch minigame", wire.packets(t))
		}
		tradeDo(t, s, c, []byte{57, 1, result})
		p := wire.packets(t)
		want := uint16(20364)
		if result == 1 {
			want = 20363
		}
		found := false
		for _, packet := range p {
			if len(packet) == 18 && packet[6] == 1 && uint16(packet[15])|uint16(packet[16])<<8 == want {
				found = true
			}
		}
		if !found {
			t.Fatal("wrong rabbit outcome dialogue", p)
		}
		for i := 0; i < 100 && c.event != nil; i++ {
			tradeDo(t, s, c, []byte{20, 6})
		}
		if c.event != nil {
			t.Fatal("rabbit did not finish")
		}
		carrots, vouchers := 0, 0
		for _, item := range c.character.Bag {
			if item.ID == 32102 {
				carrots += int(item.Count)
			}
			if item.ID == 30002 {
				vouchers += int(item.Count)
			}
		}
		expected := 0
		if result == 1 {
			expected = 10
		}
		if carrots != expected || vouchers != 0 {
			t.Fatal("wrong native reward", carrots, vouchers)
		}
		bag := c.character.Bag
		tradeDo(t, s, c, []byte{57, 1, 1})
		tradeDo(t, s, c, []byte{20, 6})
		if c.character.Bag != bag {
			t.Fatal("rabbit replay rewarded")
		}
		c.resumeAt = time.Time{}
	}
}

func TestMinigameOutcomeCanLaunchNextGame(t *testing.T) {
	s, c, wire, ev := minigameFixture(t, 3, 0)
	stale := c.event.onMinigame
	ev.Branches[1].Operations = []assets.Operation{evOp(1, 9, 4, 0x2710, 0, 0)}
	ev.Branches = append(ev.Branches, assets.Branch{Index: 4, Condition: evCond(8, 4, 0, 0, 5, 1), Operations: []assets.Operation{evOp(1, 1, 1, 2, 0, 20)}})
	wire.Reset()
	tradeDo(t, s, c, []byte{57, 1, 1})
	if c.event == nil || c.event.onMinigame == nil || !contains(wire.packets(t), []byte{57, 1, 4, 0x10, 0x27, 1}) {
		t.Fatal("new game's callback lost")
	}
	active := c.event
	if err := stale(1); err != nil || c.event != active || c.character.Gold != 0 {
		t.Fatal("old callback replaced new game", err)
	}
	tradeDo(t, s, c, []byte{57, 1, 1})
	if c.character.Gold != 20 || c.event != nil {
		t.Fatal("second game outcome did not run")
	}
}

func TestMinigameFullBagCannotPartiallyReward(t *testing.T) {
	s, c, wire, _ := minigameFixture(t, 3, 0)
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	bag := c.character.Bag
	wire.Reset()
	tradeDo(t, s, c, []byte{57, 1, 1})
	tradeDo(t, s, c, []byte{20, 6}) // the result's own acknowledgment
	wire.Reset()
	tradeDo(t, s, c, []byte{20, 6})
	if c.event != nil || c.character.Bag != bag || contains(wire.packets(t), []byte{20, 10}) {
		t.Fatal("full bag partially rewarded or retained interaction")
	}
}

// TestThrowStartSceneVariant: the dodgeball game's 57/1 carries its battle
// scene variant as a sixth byte; maps without one send 0.
func TestThrowStartSceneVariant(t *testing.T) {
	for _, tc := range []struct {
		mapID   uint16
		variant byte
	}{{60008, 50}, {12150, 90}, {0, 0}} {
		s, c, wire := eventFixture(t)
		if tc.mapID != 0 {
			c.character.Map = tc.mapID
		}
		ev := &assets.Event{ClickID: 10, Branches: []assets.Branch{
			{Index: 1, Operations: []assets.Operation{evOp(1, 9, 1, 14304, 1, 0)}},
		}}
		if err := s.startEvent(context.Background(), c, 5, ev, 0, false); err != nil {
			t.Fatal(err)
		}
		want := []byte{57, 1, 1, 0xe0, 0x37, 1, tc.variant}
		if packets := wire.packets(t); !contains(packets, want) {
			t.Fatalf("map %d: %x, want %x", tc.mapID, packets, want)
		}
	}
}
