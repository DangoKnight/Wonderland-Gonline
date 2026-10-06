package server

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
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
