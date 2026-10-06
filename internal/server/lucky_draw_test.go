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

func luckyDrawFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, p, w := friendFixture(t)
	s.Assets.Items[32176] = game.ItemDefinition{ID: 32176, Name: "Reward", Type: 23}
	s.Assets.LuckyDraw = assets.LuckyDrawPool{Rewards: []assets.LuckyDrawReward{{ID: 32176, Quantity: 2, Weight: 1, Slot: 1}}, TotalWeight: 1}
	for _, c := range p {
		next := c.character.Clone()
		next.Bag = game.Inventory{}
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
	}
	return s, p, w
}

func TestLuckyDrawNativePacketsLimitAndReconnect(t *testing.T) {
	s, p, w := luckyDrawFixture(t)
	c := p[0]
	ctx := context.Background()
	for i := byte(1); i <= 3; i++ {
		if err := s.dispatch(ctx, c, []byte{104, 1}); err != nil {
			t.Fatal(err)
		}
		packets := w[0].packets(t)
		addition := append([]byte{23, 5, 1, 176, 125, 2, 0}, make([]byte, 26)...)
		if len(packets) != 3 || !bytes.Equal(packets[0], addition) || !bytes.Equal(packets[1], []byte{104, 1, 2, 1, i}) || !bytes.Contains(packets[2], []byte("Lucky Draw: received [Reward] x2.")) {
			t.Fatal(packets)
		}
		if c.character.LuckyDraw.Used != i || c.character.Bag[0].Count != 2*i {
			t.Fatal(c.character)
		}
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A fresh session loads the durable character; login cannot restore its draws.
	c.character = &saved[0]
	if err := s.dispatch(ctx, c, []byte{104, 1}); err != nil {
		t.Fatal(err)
	}
	packets := w[0].packets(t)
	if len(packets) != 2 || !bytes.Contains(packets[0], []byte("all three")) || !bytes.Equal(packets[1], []byte{104, 1, 1, 3, 1, 176, 125, 2}) || c.character.Bag[0].Count != 6 {
		t.Fatal(packets)
	}
	for _, wire := range w[1:] {
		if wire.Len() != 0 {
			t.Fatal("private draw leaked")
		}
	}
}

func TestLuckyDrawFailuresRetainAllowanceAndFailedReceiptStaysDurable(t *testing.T) {
	s, p, w := luckyDrawFixture(t)
	c := p[0]
	ctx := context.Background()
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := *c.character
	if err := s.dispatch(ctx, c, []byte{104, 1}); err != nil || c.character.LuckyDraw != before.LuckyDraw || c.character.Bag != before.Bag {
		t.Fatal("full bag consumed draw", err)
	}
	packets := w[0].packets(t)
	if len(packets) != 1 || !bytes.Contains(packets[0], []byte("retained")) {
		t.Fatal(packets)
	}
	next.Bag = game.Inventory{}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.dispatch(canceled, c, []byte{104, 1}); err == nil || c.character.LuckyDraw.Used != 0 || !c.character.Bag[0].Empty() || w[0].Len() != 0 {
		t.Fatal("failed save acknowledged", err)
	}
	c.conn = &failedWorldConn{}
	if err := s.dispatch(ctx, c, []byte{104, 1}); err == nil {
		t.Fatal("failed write ignored")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].LuckyDraw.Used != 1 || saved[0].Bag[0].Count != 2 || c.character.LuckyDraw.Used != 1 {
		t.Fatal("receipt failure undid draw", err)
	}
}

func TestLuckyDrawInteractionGatesAndMalformedPackets(t *testing.T) {
	for _, gate := range []string{"loading", "warp", "battle", "trade", "event", "storm", "beach", "minigame"} {
		t.Run(gate, func(t *testing.T) {
			s, p, w := luckyDrawFixture(t)
			c := p[0]
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
				c.event = &eventSession{}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			case "minigame":
				c.event = &eventSession{onMinigame: func(byte) error { return nil }}
			}
			err := s.dispatch(context.Background(), c, []byte{104, 1})
			if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) {
				t.Fatal(err)
			}
			if gate != "loading" && err != nil {
				t.Fatal(err)
			}
			if c.character.LuckyDraw.Used != 0 || !c.character.Bag[0].Empty() {
				t.Fatal("gated draw")
			}
			for _, packet := range w[0].packets(t) {
				if packet[0] == 104 || (packet[0] == 23 && len(packet) > 1 && packet[1] == 5) {
					t.Fatal("gated success", packet)
				}
			}
		})
	}
	s, p, w := luckyDrawFixture(t)
	for _, packet := range [][]byte{{104}, {104, 1, 0}, {104, 2}} {
		if err := s.dispatch(context.Background(), p[0], packet); err == nil || p[0].character.LuckyDraw.Used != 0 || w[0].Len() != 0 {
			t.Fatal(packet, err)
		}
	}
	s.Assets.LuckyDraw = assets.LuckyDrawPool{}
	if err := s.dispatch(context.Background(), p[0], []byte{104, 1}); err != nil || p[0].character.LuckyDraw.Used != 0 {
		t.Fatal(err)
	}
}

func TestLuckyDrawMidnightRefreshAndClockReset(t *testing.T) {
	s, p, w := luckyDrawFixture(t)
	c := p[0]
	today := time.Date(2026, 10, 2, 23, 59, 59, 0, time.UTC)
	for i := 0; i < 3; i++ {
		s.worldMu.Lock()
		err := s.drawLucky(context.Background(), c, today)
		s.worldMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		w[0].Reset()
	}
	saved := c.character.LuckyDraw
	tomorrow := today.Add(time.Second)
	p[1].ready = false // Loading characters get synchronized on their next arrival.
	failed := &failedWorldConn{}
	p[2].conn = failed
	s.refreshLuckyDrawCatalog(tomorrow)
	packets := w[0].packets(t)
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{104, 1, 1, 0, 1, 176, 125, 2}) || c.character.LuckyDraw != saved || w[1].Len() != 0 || !failed.closed {
		t.Fatal(packets, "midnight refresh")
	}
	s.worldMu.Lock()
	err := s.drawLucky(context.Background(), c, tomorrow)
	s.worldMu.Unlock()
	if err != nil || c.character.LuckyDraw.Used != 1 || c.character.Bag[0].Count != 8 {
		t.Fatal(err, c.character)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.runLuckyDrawResets(ctx)
}

func TestLuckyDrawLoginSyncUsesSavedAllowanceBeforeWorldReady(t *testing.T) {
	s, p, w := luckyDrawFixture(t)
	c := p[0]
	next := c.character.Clone()
	next.LuckyDraw = game.LuckyDrawState{Day: time.Now().UTC().Format(time.DateOnly), Used: 2}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	session := &Session{conn: w[0], info: c.info, account: c.account}
	if err := s.dispatch(context.Background(), session, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	packets := w[0].packets(t)
	creditIndex, readyIndex := -1, -1
	for i, packet := range packets {
		if bytes.Equal(packet, []byte{104, 1, 1, 2, 1, 176, 125, 2}) {
			creditIndex = i
		}
		if bytes.Equal(packet, []byte{1, 11}) {
			readyIndex = i
		}
	}
	if creditIndex < 0 || readyIndex <= creditIndex || session.character.LuckyDraw.Used != 2 {
		t.Fatal("login credits ordering", packets)
	}
}

func TestLuckyDrawCatalogAndRefreshPreserveSavedUsage(t *testing.T) {
	s, players, wires := luckyDrawFixture(t)
	c := players[0]
	s.Assets.Items[30002] = game.ItemDefinition{ID: 30002, Name: "Another reward", Type: 23}
	s.Assets.LuckyDraw = assets.LuckyDrawPool{Rewards: []assets.LuckyDrawReward{{ID: 32176, Quantity: 2, Weight: 1, Slot: 1}, {ID: 30002, Quantity: 3, Weight: 1, Slot: 2}}, TotalWeight: 2}
	next := c.character.Clone()
	next.LuckyDraw = game.LuckyDrawState{Day: time.Now().UTC().Format(time.DateOnly), Used: 2}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	c.character = &saved[0]
	before := c.character.Clone()
	// A native menu refresh is read-only and also runs while the map is loading.
	c.ready = false
	for _, request := range [][]byte{{5, 7, 0}, {5, 4}} {
		if err := s.dispatch(context.Background(), c, request); err != nil {
			t.Fatal(err)
		}
		packets := wires[0].packets(t)
		found := false
		for _, p := range packets {
			if p[0] == 104 {
				if !bytes.Equal(p, []byte{104, 1, 1, 2, 2, 176, 125, 2, 50, 117, 3}) {
					t.Fatal("native catalog bytes", p)
				}
				found = true
			}
			if p[0] == 23 {
				t.Fatal("refresh granted inventory", p)
			}
		}
		if !found || c.character.LuckyDraw != before.LuckyDraw || c.character.Bag != before.Bag {
			t.Fatal("refresh lost saved usage")
		}
	}
}
