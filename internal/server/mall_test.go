package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

func gachaFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := worldFixture(t)
	s.Assets.Items[34171] = game.ItemDefinition{ID: 34171, Name: "Gift Pack", Type: 23}
	s.Assets.Items[34333] = game.ItemDefinition{ID: 34333, Name: "Lucky Pack", Type: 23}
	s.Assets.Items[32176] = game.ItemDefinition{ID: 32176, Name: "Reward", Type: 23}
	s.Assets.GachaPacks = map[uint16]assets.GachaPack{34171: {ID: 34171, Rewards: []assets.GachaReward{{ID: 32176, Weight: 10000, Quantity: 3}}}}
	for _, c := range players {
		if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	next := players[0].character.Clone()
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 34171, Count: 2, Damage: 7, Metadata: [26]byte{8, 9}}
	next.Bag[8] = game.Item{ID: 34171, Count: 1}
	if err := s.commit(context.Background(), players[0], next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}
func TestGachaNativeOpeningCommandsAndExactSlot(t *testing.T) {
	for _, request := range [][]byte{{23, 75, 9, 0}, {23, 128, 9, 0}, {23, 96, 9}} {
		t.Run(fmt.Sprintf("AC23:%d", request[1]), func(t *testing.T) {
			s, players, wires := gachaFixture(t)
			c := players[0]
			before := c.character.Bag[4]
			if err := s.dispatch(context.Background(), c, request); err != nil {
				t.Fatal(err)
			}
			packets := wires[0].packets(t)
			addition := append([]byte{23, 5, 1, 176, 125, 3, 0}, make([]byte, 26)...)
			banner := append([]byte{2, 16, 0, 0, 0, 0}, []byte("Gacha: received [Reward] x3.")...)
			want := [][]byte{{23, 9, 9, 1}, addition, {23, 15}, banner}
			if len(packets) != len(want) {
				t.Fatal(packets)
			}
			for i := range want {
				if !bytes.Equal(packets[i], want[i]) {
					t.Fatal(packets[i], want[i])
				}
			}
			if c.character.Bag[4] != before || !c.character.Bag[8].Empty() || c.character.Bag[0] != (game.Item{ID: 32176, Count: 3}) {
				t.Fatal("exact slot or fresh reward metadata", c.character.Bag)
			}
			if wires[1].Len() != 0 || wires[2].Len() != 0 {
				t.Fatal("private pack result broadcast")
			}
			chars, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || chars[0].Bag != c.character.Bag {
				t.Fatal("not persisted", err)
			}
			saved := c.character.Bag
			if err := s.dispatch(context.Background(), c, request); err != nil || c.character.Bag != saved {
				t.Fatal("empty-slot replay", err)
			}
			for _, p := range wires[0].packets(t) {
				if p[0] == 23 {
					t.Fatal("replay published success", p)
				}
			}
		})
	}
}
func TestGachaPreviewIsReadOnlyAndIgnoresCache(t *testing.T) {
	s, players, wires := gachaFixture(t)
	c := players[0]
	next := c.character.Clone()
	next.Bag = game.Inventory{}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	c.trade = &tradeSession{} // Read-only previews remain available while a trade is open.
	before := c.character.Bag
	s.Assets.GachaPacks[34171] = assets.GachaPack{ID: 34171, Rewards: []assets.GachaReward{{ID: 100, Quantity: 1, Weight: 6000}, {ID: 200, Quantity: 2, Weight: 4000}}}
	for _, version := range []byte{0, 1, 255} {
		if err := s.dispatch(context.Background(), c, []byte{91, 1, 123, 133, version}); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		want := []byte{91, 2, 123, 133, 1, 100, 0, 1, 200, 0, 2}
		if len(got) != 1 || !bytes.Equal(got[0], want) || c.character.Bag != before {
			t.Fatal(got)
		}
	}
	for _, id := range []uint16{34333, 1234} {
		if err := s.dispatch(context.Background(), c, protocol.Builder{91, 1}.U16(id).U8(1)); err != nil {
			t.Fatal(err)
		}
		got := wires[0].packets(t)
		want := protocol.Builder{91, 2}.U16(id).U8(1)
		if len(got) != 1 || !bytes.Equal(got[0], want) {
			t.Fatal(got)
		}
	}
	if c.trade == nil {
		t.Fatal("preview canceled the trade")
	}
	c.trade = nil
	for _, p := range [][]byte{{91, 1}, {91, 1, 123, 133}, {91, 1, 123, 133, 1, 0}, {23, 75, 9}, {23, 128, 9, 0, 0}} {
		if err := s.dispatch(context.Background(), c, p); err == nil || c.character.Bag != before || wires[0].Len() != 0 {
			t.Fatal("malformed preview/open", p, err)
		}
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("preview leaked")
	}
}
func TestGachaUnavailableAndLargeSlotsRetainPack(t *testing.T) {
	s, players, wires := gachaFixture(t)
	c := players[0]
	before := c.character.Bag
	for _, slot := range []uint16{0, 51, 265, 65535} {
		if err := s.dispatch(context.Background(), c, protocol.Builder{23, 75}.U16(slot)); err != nil || c.character.Bag != before || wires[0].Len() != 0 {
			t.Fatal("invalid slot", slot, err)
		}
	}
	delete(s.Assets.GachaPacks, 34171)
	if err := s.dispatch(context.Background(), c, []byte{23, 75, 9, 0}); err != nil || c.character.Bag != before {
		t.Fatal(err)
	}
	p := wires[0].packets(t)
	if len(p) != 1 || !bytes.Contains(p[0], []byte("unavailable")) {
		t.Fatal(p)
	}
	next := c.character.Clone()
	next.Bag[8] = game.Item{ID: 34333, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(context.Background(), c, []byte{23, 96, 9}); err != nil || c.character.Bag[8].Count != 1 {
		t.Fatal("withdrawn pack consumed", err)
	}
}
func TestGachaFullBagAndFailedSave(t *testing.T) {
	s, players, wires := gachaFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	next.Bag[8] = game.Item{ID: 34171, Count: 2}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Bag
	if err := s.dispatch(ctx, c, []byte{23, 75, 9, 0}); err != nil || c.character.Bag != before {
		t.Fatal("full bag consumed pack", err)
	}
	p := wires[0].packets(t)
	if len(p) != 1 || !bytes.Contains(p[0], []byte("free an inventory slot")) {
		t.Fatal(p)
	}
	next = c.character.Clone()
	next.Bag[8].Count = 1
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before = c.character.Bag
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.dispatch(canceled, c, []byte{23, 75, 9, 0}); err == nil || c.character.Bag != before || wires[0].Len() != 0 {
		t.Fatal("failed save published success", err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != before {
		t.Fatal(chars, err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 75, 9, 0}); err != nil || c.character.Bag[8] != (game.Item{ID: 32176, Count: 3}) {
		t.Fatal("freed pack slot not reused", err)
	}
}
func TestGachaQuantitySplitsAndMetadataStackIsolation(t *testing.T) {
	s, players, wires := gachaFixture(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.GachaPacks[34171] = assets.GachaPack{ID: 34171, Rewards: []assets.GachaReward{{ID: 32176, Weight: 10000, Quantity: 255}}}
	next := c.character.Clone()
	next.Bag[0] = game.Item{ID: 32176, Count: 40}
	next.Bag[1] = game.Item{ID: 32176, Count: 49, Damage: 2, Metadata: [26]byte{5}}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 75, 9, 0}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[0].Count != 50 || c.character.Bag[1] != next.Bag[1] || c.character.Bag[4] != next.Bag[4] {
		t.Fatal("metadata or exact slot", c.character.Bag)
	}
	count := 0
	for _, item := range c.character.Bag {
		if item.ID == 32176 && item.Damage == 0 {
			count += int(item.Count)
			if item.Count > 50 {
				t.Fatal("stack overflow")
			}
		}
	}
	if count != 295 {
		t.Fatal(count)
	}
	p := wires[0].packets(t)
	if len(p) != 4 || len(p[1]) != 2+6*31 {
		t.Fatal("split addition receipts", p)
	}
}
func TestGachaGameplayGates(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "event", "minigame", "storm", "beach", "trade", "mounted"} {
		t.Run(gate, func(t *testing.T) {
			s, players, wires := gachaFixture(t)
			c := players[0]
			before := c.character.Bag
			switch gate {
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "event":
				c.event = &eventSession{}
			case "minigame":
				c.event = &eventSession{onMinigame: func(byte) error { return nil }}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			case "trade":
				c.trade = &tradeSession{}
			case "mounted":
				c.character.ActiveVehicle = 34171
				c.character.VehicleSlot = 9
			}
			err := s.dispatch(context.Background(), c, []byte{23, 75, 9, 0})
			if gate == "loading" && err == nil {
				t.Fatal("pre-ack opened")
			}
			if c.character.Bag != before || wires[1].Len() != 0 || wires[2].Len() != 0 {
				t.Fatal("gated mutation", err)
			}
			for _, p := range wires[0].packets(t) {
				if p[0] == 23 && p[1] != 57 {
					t.Fatal("gated success", p)
				}
			}
		})
	}
}
func TestGachaMallAvailabilityAndOpening(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	s.Assets.Items[34171] = game.ItemDefinition{ID: 34171, Type: 23}
	s.Assets.Mall = []assets.MallItem{{ID: 34171, Count: 1, Cost: 10, CategoryID: 4, Order: 12}, {ID: 34333, Count: 1, Cost: 1}}
	if len(s.mallEntries(false)) != 0 {
		t.Fatal("unconfigured pack advertised")
	}
	s.Assets.GachaPacks = map[uint16]assets.GachaPack{34171: {ID: 34171, Rewards: []assets.GachaReward{{ID: 32176, Weight: 10000, Quantity: 3}}}}
	if entries := s.mallEntries(false); len(entries) != 1 || entries[0].ID != 34171 {
		t.Fatal(entries)
	}
	if err := s.dispatch(ctx, c, mallCart(1, mallCartRow{item: 34171, category: 4, quantity: 1, order: 12})); err != nil || c.character.Bag[0].ID != 34171 || c.account.IM != 90 {
		t.Fatal("pack purchase", err)
	}
	wire.Reset()
	if err := s.dispatch(ctx, c, []byte{23, 128, 1, 0}); err != nil || c.character.Bag[0] != (game.Item{ID: 32176, Count: 3}) || c.account.IM != 90 {
		t.Fatal("purchased pack open", err)
	}
}

func TestGachaSQLDefinedPackUsesAllNativeOpenPaths(t *testing.T) {
	for _, request := range [][]byte{{23, 75, 9, 0}, {23, 128, 9, 0}, {23, 96, 9}} {
		t.Run(fmt.Sprintf("AC23:%d", request[1]), func(t *testing.T) {
			s, players, wires := gachaFixture(t)
			c := players[0]
			s.Assets.Items[64000] = game.ItemDefinition{ID: 64000, Name: "Configured Pack", Type: 23}
			s.Assets.GachaPacks[64000] = assets.GachaPack{ID: 64000, Rewards: []assets.GachaReward{{ID: 32176, Weight: 10000, Quantity: 3}}}
			next := c.character.Clone()
			next.Bag[8] = game.Item{ID: 64000, Count: 1}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			if err := s.dispatch(context.Background(), c, []byte{91, 1, 0, 250, 0}); err != nil {
				t.Fatal(err)
			}
			preview := wires[0].packets(t)
			if len(preview) != 1 || !bytes.Equal(preview[0], []byte{91, 2, 0, 250, 1, 176, 125, 3}) {
				t.Fatal(preview)
			}
			if err := s.dispatch(context.Background(), c, request); err != nil {
				t.Fatal(err)
			}
			if !c.character.Bag[8].Empty() || c.character.Bag[0] != (game.Item{ID: 32176, Count: 3}) {
				t.Fatal(c.character.Bag)
			}
			packets := wires[0].packets(t)
			if len(packets) != 4 || !bytes.Equal(packets[0], []byte{23, 9, 9, 1}) || !bytes.Equal(packets[2], []byte{23, 15}) {
				t.Fatal(packets)
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Bag != c.character.Bag {
				t.Fatal("durability", err)
			}
		})
	}
}

func TestGachaUnavailableConfiguredPackCannotBeSoldOrConsumed(t *testing.T) {
	s, players, wires := gachaFixture(t)
	c := players[0]
	s.Assets.Items[64000] = game.ItemDefinition{ID: 64000, Type: 23}
	s.Assets.UnavailableGachaPacks = map[uint16]assets.GachaPackIssue{64000: {ID: 64000, MissingItems: []uint16{63000}}}
	s.Assets.Mall = []assets.MallItem{{ID: 64000, Count: 1, Cost: 10}}
	if len(s.mallEntries(false)) != 0 {
		t.Fatal("disabled custom pack advertised")
	}
	next := c.character.Clone()
	next.Bag[8] = game.Item{ID: 64000, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Bag
	if err := s.dispatch(context.Background(), c, []byte{23, 96, 9}); err != nil || c.character.Bag != before {
		t.Fatal("disabled custom pack consumed", err)
	}
	packets := wires[0].packets(t)
	if len(packets) != 1 || !bytes.Contains(packets[0], []byte("unavailable")) {
		t.Fatal(packets)
	}
	if err := s.dispatch(context.Background(), c, []byte{91, 1, 0, 250, 0}); err != nil {
		t.Fatal(err)
	}
	preview := wires[0].packets(t)
	if len(preview) != 1 || !bytes.Equal(preview[0], []byte{91, 2, 0, 250, 1}) {
		t.Fatal(preview)
	}
}

func TestGachaConfiguredPackMallPurchaseAndOpening(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	s.Assets.Items[64000] = game.ItemDefinition{ID: 64000, Type: 23}
	s.Assets.Mall = []assets.MallItem{{ID: 64000, Count: 1, Cost: 10, CategoryID: 4, Order: 12}}
	s.Assets.GachaPacks = map[uint16]assets.GachaPack{64000: {ID: 64000, Rewards: []assets.GachaReward{{ID: 32176, Weight: 10000, Quantity: 3}}}}
	if entries := s.mallEntries(false); len(entries) != 1 || entries[0].ID != 64000 {
		t.Fatal(entries)
	}
	if err := s.dispatch(ctx, c, mallCart(1, mallCartRow{item: 64000, category: 4, quantity: 1, order: 12})); err != nil || c.character.Bag[0].ID != 64000 || c.account.IM != 90 {
		t.Fatal("configured pack purchase", err)
	}
	wire.Reset()
	if err := s.dispatch(ctx, c, []byte{23, 128, 1, 0}); err != nil || c.character.Bag[0] != (game.Item{ID: 32176, Count: 3}) || c.account.IM != 90 {
		t.Fatal("purchased configured pack open", err)
	}
}

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

func expectMallAdjustment(t *testing.T, wire *captureConn, points, bonus byte) {
	t.Helper()
	packets := wire.packets(t)
	// Independent native AC75:3 and AC75:9 layout: balance, spent, item, count.
	want := [][]byte{{75, 3, points, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, {75, 9, bonus, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
	if len(packets) != 2 {
		t.Fatal(packets)
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Fatal(packets[i], want[i])
		}
	}
}

func TestGMMallPointCommandsAndTargeting(t *testing.T) {
	s, players, wires := chatFixture(t)
	gm, target := players[0], players[1]
	ctx := context.Background()
	say(t, s, gm, ":im 10 Bobby")
	if balance, err := s.Store.MallBalances(ctx, target.account.ID); err != nil || balance.Points != 0 {
		t.Fatal("non-GM granted currency", balance, err)
	}
	s.SetGMLevel(gm.account.ID, 1)
	for _, command := range []string{":im 10 Bobby", "/points_im 10 Bobby", "/mallpoints 10 Bobby"} {
		say(t, s, gm, command)
	}
	got := wires[1].packets(t)
	if len(got) != 6 {
		t.Fatal(got)
	}
	if balance, err := s.Store.MallBalances(ctx, target.account.ID); err != nil || balance.Points != 30 || target.account.IM != 30 {
		t.Fatal(balance, err)
	}
	for _, command := range []string{":im 10 nobody", ":im bad", ":im 2147483648", ":im 0", ":im"} {
		say(t, s, gm, command)
	}
	if wires[0].Len() != 0 || wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("invalid target granted to self or leaked receipts")
	}
	say(t, s, gm, ":im 15")
	expectMallAdjustment(t, wires[0], 15, 0)
	say(t, s, gm, ":mallpoints -100 Bobby")
	expectMallAdjustment(t, wires[1], 0, 0)
	if target.account.IM != 0 {
		t.Fatal("deduction did not floor at zero")
	}
}

func TestAdminMallAdjustmentLiveLoadingAndFailure(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	ctx := context.Background()
	if _, err := s.AdjustMallBalance(ctx, c.account.ID, true, 17); err != nil {
		t.Fatal(err)
	}
	expectMallAdjustment(t, wires[0], 0, 17)
	if wires[1].Len() != 0 || wires[2].Len() != 0 || c.account.IMBonus != 17 {
		t.Fatal("balance leaked or cache stale")
	}
	// Defer an adjustment while this connection is loading its next map.
	s.worldMu.Lock()
	c.ready = false
	delete(s.world, c.info.ID)
	s.worldMu.Unlock()
	if _, err := s.AdjustMallBalance(ctx, c.account.ID, false, 23); err != nil {
		t.Fatal(err)
	}
	if wires[0].Len() != 0 || c.account.IM != 0 {
		t.Fatal("interleaved loading snapshot")
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	if len(packets) < 2 || !bytes.Equal(packets[0], []byte{75, 3, 23, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) || !bytes.Equal(packets[1], []byte{75, 9, 17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) || c.account.IM != 23 {
		t.Fatal("loading grant not synchronized", packets)
	}
	for _, w := range wires {
		w.Reset()
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.AdjustMallBalance(canceled, c.account.ID, false, 1); err == nil || c.account.IM != 23 || wires[0].Len() != 0 {
		t.Fatal("failed grant published", err)
	}
	failed := &failedWorldConn{}
	c.conn = failed
	balance, err := s.AdjustMallBalance(ctx, c.account.ID, false, 1)
	if err != nil || balance.Points != 24 || !failed.closed {
		t.Fatal("committed grant reported failure after receipt error", balance, err)
	}
	if current, err := s.Store.MallBalances(ctx, c.account.ID); err != nil || current != balance {
		t.Fatal(current, err)
	}
}

func TestAdminMallAdjustmentOrdersConcurrentRequests(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.AdjustMallBalance(context.Background(), c.account.ID, false, 1)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	packets := wires[0].packets(t)
	if len(packets) != 20 || c.account.IM != 10 {
		t.Fatal(packets)
	}
	for i := 0; i < 10; i++ {
		if packets[i*2][2] != byte(i+1) {
			t.Fatal("balance receipts reordered", packets)
		}
	}
}

func TestMallAdjustmentPurchaseUsesSavedGrant(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	// Ensure publication so administrator receipts share the purchase order.
	s.world[c.info.ID] = c
	if _, err := s.AdjustMallBalance(ctx, c.account.ID, false, -100); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := s.worldCommand(ctx, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12})); err != nil {
		t.Fatal(err)
	}
	if !c.character.Bag[0].Empty() || c.account.IM != 0 {
		t.Fatal("purchase used stale pre-adjustment funds")
	}
	wire.Reset()
	if _, err := s.AdjustMallBalance(ctx, c.account.ID, false, 10); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := s.worldCommand(ctx, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12})); err != nil || c.character.Bag[0] != (game.Item{ID: 32176, Count: 5}) || c.account.IM != 0 {
		t.Fatal("grant not spendable", err)
	}
	if balance, err := s.Store.MallBalances(ctx, c.account.ID); err != nil || balance != (store.MallBalances{Points: 0, Bonus: 30}) {
		t.Fatal(balance, err)
	}
}

func TestMallCheckoutNativeGolden(t *testing.T) {
	packets := serverwire.MallCheckoutPackets(store.MallBalances{Points: 0x12345678, Bonus: 0x11223344})
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
			if gate == "trade" || gate == "minigame" {
				// The read-only query is answered without cancelling the trade or
				// consuming the minigame; the fruit machines (sport kinds 11, 12
				// and 14) send it while they run.
				if packets := wire.packets(t); len(packets) != 3 || packets[2][0] != 35 || packets[2][1] != 4 || (gate == "trade" && c.trade == nil) || (gate == "minigame" && c.event == nil) {
					t.Fatal("read-only checkout changed the interaction", packets)
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
		packets := serverwire.MallCheckoutPackets(store.MallBalances{Points: test.value, Bonus: test.value})
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
