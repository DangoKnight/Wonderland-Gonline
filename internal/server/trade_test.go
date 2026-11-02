package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func TestBazaarNativeBrowseQuotedPurchaseAndPetVehicleLayouts(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	c.character.Gold = 500
	c.character.Bag = game.Inventory{}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	b, err := s.Store.SaveBazaar(ctx, store.Bazaar{Name: "Merchant", Title: "Goods", Map: c.character.Map, X: c.character.X, Y: c.character.Y, Kind: store.BazaarItems, Listings: []store.BazaarListing{{ItemID: 32176, Quantity: 3, Price: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	spawn := bazaarPackets(b)
	if len(spawn[0]) == 0 || spawn[0][0] != 4 {
		t.Fatal("invalid actor", spawn)
	}
	request := protocol.Builder{23, 33}.U32(b.VendorID).U8(0).U8(2).U16(32176).U32(10)
	if err = s.nativeStallCommand(ctx, c, request); err != nil || c.character.Gold != 500 {
		t.Fatal("purchase without browse", err)
	}
	wire.Reset()
	if err = s.nativeStallCommand(ctx, c, protocol.Builder{23, 32}.U32(b.VendorID)); err != nil {
		t.Fatal(err)
	}
	got := wire.packets(t)
	if len(got) != 1 || len(got[0]) != 58 || got[0][19] != 0 || !bytes.Equal(got[0][20:28], []byte{176, 125, 3, 50, 10, 0, 0, 0}) {
		t.Fatal("item browse", got)
	}
	if err = s.nativeStallCommand(ctx, c, request); err != nil || c.character.Gold != 480 || c.character.Bag[0].Count != 2 {
		t.Fatal(c.character.Bag, err)
	}
	saved, _ := s.Store.Bazaar(ctx, b.VendorID)
	if saved.Listings[0].Quantity != 1 {
		t.Fatal(saved)
	}
	b.Kind = store.BazaarVehicles
	b.Listings = []store.BazaarListing{{VendorID: b.VendorID, ItemID: 48010, Quantity: 1, Price: 10}}
	want := []byte{23, 133, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 138, 187, 100, 100, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(want[2:], b.VendorID)
	if packet := s.bazaarBrowsePacket(b); !bytes.Equal(packet, want) {
		t.Fatal("vehicle browse", packet)
	}
	s.Assets.NPCs = map[uint16]assets.NPC{17162: {ID: 17162, Name: "S.Monkey", Type: 1, Level: 1, Element: 3, Stats: [5]uint16{10, 11, 12, 13, 14}}}
	s.World = world.New(s.Assets)
	b.Kind = store.BazaarPets
	b.Listings = []store.BazaarListing{{VendorID: b.VendorID, ItemID: 17162, Quantity: 2, Price: 30}}
	b, err = s.Store.SaveBazaar(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	packet := s.bazaarBrowsePacket(b)
	if len(packet) != 231 || !bytes.Equal(packet[:2], []byte{23, 113}) || !bytes.Equal(packet[6:14], []byte{0, 10, 67, 30, 0, 0, 0, 1}) || !bytes.Equal(packet[22:24], []byte{3, 0}) {
		t.Fatal("pet browse", packet)
	}
	if err = s.browseBazaar(ctx, c, b.VendorID); err != nil {
		t.Fatal(err)
	}
	if err = s.bazaarTypedPurchase(ctx, c, protocol.Builder{23, 63}.U32(b.VendorID).U8(0)); err != nil || len(c.character.Pets) != 1 || c.character.Pets[0].ID != 17162 || c.character.Gold != 450 {
		t.Fatal(c.character.Pets, err)
	}
}

func TestNativeStallBrowsePurchaseAndQuote(t *testing.T) {
	s, players, wires := tradeFixture(t)
	seller, buyer := players[0], players[1]
	open, _ := protocol.Builder{23, 30}.String("Goods")
	open = open.U8(1).U8(1).U8(6).U32(3)
	tradeDo(t, s, seller, open)
	if seller.stall == nil || !seller.stall.Native {
		t.Fatal("native stall not opened")
	}
	for _, w := range wires {
		w.Reset()
	}
	tradeDo(t, s, buyer, protocol.Builder{23, 32}.U32(seller.character.ID))
	got := wires[1].packets(t)
	if len(got) != 2 || len(got[0]) != 58 || got[0][0] != 23 || got[0][1] != 33 || got[0][19] != 0 || got[0][20] != 0xb0 || got[0][21] != 0x7d || got[0][22] != 6 || got[0][24] != 3 {
		t.Fatal("native browse record", got)
	}
	buy := func(price uint32) []byte {
		return protocol.Builder{23, 33}.U32(seller.character.ID).U8(0).U8(2).U16(32176).U32(price)
	}
	tradeDo(t, s, buyer, buy(4))
	if seller.character.Gold != 100 || buyer.character.Gold != 200 {
		t.Fatal("stale quote charged")
	}
	tradeDo(t, s, buyer, buy(3))
	if seller.character.Gold != 106 || buyer.character.Gold != 194 || seller.stall.Listings[0].Item.Count != 4 || buyer.character.Bag[1].Count != 2 || buyer.character.Bag[1].Metadata[6] != 7 {
		t.Fatal("native stall exchange")
	}
	stored := tradeStored(t, s, players)
	if stored[0].Gold != 106 || stored[1].Gold != 194 || stored[1].Bag != buyer.character.Bag {
		t.Fatal("native exchange not atomic")
	}
	if err := s.dispatch(context.Background(), seller, []byte{23, 31}); err != nil || seller.stall != nil {
		t.Fatal("native close", err)
	}
}

func TestAlternativeStallAdaptersUseOwnedInventory(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	c.character.Bag = game.Inventory{}
	if err := s.bazaarCommand(ctx, c, []byte{53, 1}); err != nil || c.stall != nil {
		t.Fatal("invented empty-bag offers", err)
	}
	c.character.Bag[0] = game.Item{ID: 32176, Count: 3}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	request, _ := protocol.Builder{53, 1}.String("Goods")
	if err := s.bazaarCommand(ctx, c, request); err != nil || c.stall == nil || len(c.stall.Listings) != 1 || c.stall.Listings[0].Item.Slot != 1 || c.stall.Listings[0].Item.Item.ID != 32176 {
		t.Fatal(c.stall, err)
	}
	wire.Reset()
	if err := s.eventStallCommand(ctx, c, []byte{20, 13}); err != nil || c.stall != nil {
		t.Fatal("event stall close", err)
	}
}

func TestNativeStallFiveByteOffersPreserveStyleSlotsAndMetadata(t *testing.T) {
	s, players, wires := tradeFixture(t)
	seller, buyer := players[0], players[1]
	// Native title, style two, then one five-byte (bag slot, price) record.
	open := []byte{23, 30, 5, 'G', 'o', 'o', 'd', 's', 2, 1, 3, 0, 0, 0}
	tradeDo(t, s, seller, open)
	if seller.stall == nil || !seller.stall.NativeSlots || seller.stall.Style != 2 || seller.stall.Listings[0].Item.Count != 10 {
		t.Fatal("native offer lost style, bag identity or whole-stack quantity")
	}
	if !contains(wires[0].packets(t), open) {
		t.Fatal("native open acknowledgment changed")
	}
	for _, w := range wires {
		w.Reset()
	}
	tradeDo(t, s, buyer, protocol.Builder{23, 32}.U32(seller.character.ID))
	packets := wires[1].packets(t)
	if len(packets) != 2 || len(packets[0]) != 58 || packets[0][19] != 1 || packets[0][22] != 10 {
		t.Fatal("native browse must use the original bag slot", packets)
	}
	tradeDo(t, s, buyer, protocol.Builder{23, 33}.U32(seller.character.ID).U8(1).U8(2).U16(32176).U32(3))
	stored := tradeStored(t, s, players)
	if stored[0].Gold != 106 || stored[1].Gold != 194 || stored[0].Bag[0].Count != 8 || stored[1].Bag[1].Count != 2 || stored[1].Bag[1].Damage != 2 || stored[1].Bag[1].Metadata[6] != 7 {
		t.Fatal("native purchase lost ownership, funds or instance metadata")
	}
}

func TestStallListDuringMapLoadingAndGameplay(t *testing.T) {
	for _, ready := range []bool{false, true} {
		name := "loading"
		if ready {
			name = "ready"
		}
		t.Run(name, func(t *testing.T) {
			s := New(config.Default(), nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			wire := &captureConn{}
			c := &Session{conn: wire, character: &game.Character{ID: 10001, Map: 10017}, ready: ready}
			before := c.character.Clone()
			for range 2 {
				if err := s.dispatch(context.Background(), c, []byte{23, 77}); err != nil {
					t.Fatal(err)
				}
				got := wire.packets(t)
				if len(got) != 2 || !bytes.Equal(got[0], []byte{23, 4, 0}) || !bytes.Equal(got[1], []byte{23, 102}) {
					t.Fatalf("stall list replies %x", got)
				}
			}
			if err := s.dispatch(context.Background(), c, []byte{5, 7, 0}); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			if len(got) != 1 || !bytes.Equal(got[0], []byte{5, 8, 0x11, 0x27, 0, 0, 0}) {
				t.Fatalf("sprite refresh %x", got)
			}
			if err := s.dispatch(context.Background(), c, []byte{5, 4}); err != nil {
				t.Fatal(err)
			}
			if got := wire.packets(t); len(got) != 0 {
				t.Fatalf("state ping unexpectedly echoed %x", got)
			}
			if c.ready != ready || c.character.ID != before.ID || c.character.Map != before.Map || c.character.Bag != before.Bag {
				t.Fatal("read-only request changed world or inventory state")
			}
			if !ready {
				if err := s.dispatch(context.Background(), c, []byte{5, 17, 1}); !errors.Is(err, protocol.ErrMalformed) {
					t.Fatalf("teleport allowed before map acknowledgment: %v", err)
				}
				for _, request := range [][]byte{{23, 26, 1, 0}, {75, 1, 1, 1, 0, 4, 1, 1, 0}} {
					if err := s.dispatch(context.Background(), c, request); !errors.Is(err, protocol.ErrMalformed) {
						t.Fatalf("mall purchase allowed before map acknowledgment: %v", err)
					}
				}
				// Only read-only synchronization bypasses the map-loading guard.
				if err := s.dispatch(context.Background(), c, []byte{23, 10, 1, 1, 2}); !errors.Is(err, protocol.ErrMalformed) {
					t.Fatalf("inventory mutation allowed before map acknowledgment: %v", err)
				}
			}
		})
	}
	s := New(config.Default(), nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := s.dispatch(context.Background(), &Session{}, []byte{23, 77}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatalf("list allowed without selected character: %v", err)
	}
}

// stallOpen crafts AC23:1 with the serialized title, listing count and records.
func stallOpen(title string, listings ...[]byte) []byte {
	p, _ := protocol.Builder{protocol.CommandStall, protocol.StallOpen}.String(title)
	p = p.U8(byte(len(listings)))
	for _, row := range listings {
		p = p.Bytes(row)
	}
	return p
}

func stallRecord(slot byte, id uint16, price uint32, count byte) []byte {
	return protocol.Builder{}.U8(slot).U16(id).U32(price).U8(count)
}

// TestStallOpenRejectsMalformedOffers covers every rejected shape of AC23:1.
func TestStallOpenRejectsMalformedOffers(t *testing.T) {
	s, players, _ := tradeFixture(t)
	seller := players[0]
	cases := map[string][]byte{
		"empty title":      stallOpen("", stallRecord(1, 32176, 10, 5)),
		"no listings":      stallOpen("Shop"),
		"slot zero":        stallOpen("Shop", stallRecord(0, 32176, 10, 5)),
		"slot too high":    stallOpen("Shop", stallRecord(game.BagSize+1, 32176, 10, 5)),
		"duplicate slot":   stallOpen("Shop", stallRecord(1, 32176, 10, 5), stallRecord(1, 32176, 10, 5)),
		"zero price":       stallOpen("Shop", stallRecord(1, 32176, 0, 5)),
		"zero quantity":    stallOpen("Shop", stallRecord(1, 32176, 10, 0)),
		"malformed header": {protocol.CommandStall, protocol.StallOpen},
	}
	for name, packet := range cases {
		t.Run(name, func(t *testing.T) {
			if err := s.dispatch(context.Background(), seller, packet); !errors.Is(err, protocol.ErrMalformed) {
				t.Fatalf("accepted malformed offer: %v", err)
			}
			if seller.stall != nil {
				t.Fatal("malformed offer opened a stall")
			}
		})
	}
}

// TestStallOpenRejectsChangedInventory checks the soft rejections that answer
// with a system message instead of closing the connection.
func TestStallOpenRejectsChangedInventory(t *testing.T) {
	s, players, wires := tradeFixture(t)
	seller := players[0]
	cases := map[string][]byte{
		"above the stack": stallOpen("Shop", stallRecord(1, 32176, 10, 11)),
		"unknown item":    stallOpen("Shop", stallRecord(1, 9999, 10, 1)),
		"empty slot":      stallOpen("Shop", stallRecord(2, 32176, 10, 1)),
		"locked item":     stallOpen("Shop", stallRecord(1, 32176, 10, 1)),
	}
	for name, packet := range cases {
		t.Run(name, func(t *testing.T) {
			if name == "locked item" {
				before := seller.character.Bag[0].Locked
				seller.character.Bag[0].Locked = true
				defer func() { seller.character.Bag[0].Locked = before }()
			}
			wires[0].Reset()
			if err := s.dispatch(context.Background(), seller, packet); err != nil {
				t.Fatalf("rejection should be a message, not an error: %v", err)
			}
			if seller.stall != nil {
				t.Fatal("rejected offer opened a stall")
			}
			if got := wires[0].packets(t); len(got) == 0 {
				t.Fatal("rejection produced no message for the seller")
			}
		})
	}
}

// TestStallCloseIsIdempotent checks that a second close is a no-op and that the
// removal packets reach peers once.
func TestStallCloseIsIdempotent(t *testing.T) {
	s, players, wires := tradeFixture(t)
	seller, peer := players[0], players[1]
	openTestStall(t, s, seller)
	for _, wire := range wires {
		wire.Reset()
	}

	tradeDo(t, s, seller, []byte{protocol.CommandStall, protocol.StallClose})
	if seller.stall != nil {
		t.Fatal("stall stayed open")
	}
	if first := len(wires[1].packets(t)); first == 0 {
		t.Fatal("closing did not notify peers")
	}

	// packets() drains the wire, so a second close must leave it empty.
	tradeDo(t, s, seller, []byte{protocol.CommandStall, protocol.StallClose})
	if got := len(wires[1].packets(t)); got != 0 {
		t.Fatalf("second close broadcast %d packet(s)", got)
	}
	if peer.stall != nil {
		t.Fatal("closing one stall touched another player")
	}
}

// TestStallViewAndPurchaseGuards covers the unavailable branches that protect
// the window from stale or invalid requests.
func TestStallViewAndPurchaseGuards(t *testing.T) {
	s, players, _ := tradeFixture(t)
	seller, buyer := players[0], players[1]
	openTestStall(t, s, seller)

	view := protocol.Builder{protocol.CommandStall, protocol.StallView}.U32(seller.character.ID)
	if err := s.dispatch(context.Background(), buyer, view); err != nil {
		t.Fatal(err)
	}

	// An unknown seller is reported as unavailable, not as a malformed packet.
	missing := protocol.Builder{protocol.CommandStall, protocol.StallView}.U32(999)
	if err := s.dispatch(context.Background(), buyer, missing); err != nil {
		t.Fatal(err)
	}

	// Buying from yourself is refused.
	self := protocol.Builder{protocol.CommandStall, protocol.StallBuy}.U32(buyer.character.ID).U8(1).U8(1)
	if err := s.dispatch(context.Background(), buyer, self); err != nil {
		t.Fatal(err)
	}

	// Buying while running a stall is refused.
	before := tradeStored(t, s, players)
	openTestStall(t, s, buyer)
	other := protocol.Builder{protocol.CommandStall, protocol.StallBuy}.U32(seller.character.ID).U8(1).U8(1)
	if err := s.dispatch(context.Background(), buyer, other); err != nil {
		t.Fatal(err)
	}
	if after := tradeStored(t, s, players); after[0].Gold != before[0].Gold || after[1].Gold != before[1].Gold {
		t.Fatal("purchase went through while the buyer ran a stall")
	}
	tradeDo(t, s, buyer, []byte{protocol.CommandStall, protocol.StallClose})

	// A missing listing and a quantity above the listed stock answer with a message.
	gone := protocol.Builder{protocol.CommandStall, protocol.StallBuy}.U32(seller.character.ID).U8(9).U8(1)
	if err := s.dispatch(context.Background(), buyer, gone); err != nil {
		t.Fatal(err)
	}
	tooMany := protocol.Builder{protocol.CommandStall, protocol.StallBuy}.U32(seller.character.ID).U8(1).U8(200)
	if err := s.dispatch(context.Background(), buyer, tooMany); err != nil {
		t.Fatal(err)
	}

	// A zero count is refused before the transaction runs.
	zero := protocol.Builder{protocol.CommandStall, protocol.StallBuy}.U32(seller.character.ID).U8(1).U8(0)
	if err := s.dispatch(context.Background(), buyer, zero); err != nil {
		t.Fatal(err)
	}
	if after := tradeStored(t, s, players); after[0].Bag != before[0].Bag || after[1].Bag != before[1].Bag {
		t.Fatal("guarded purchases changed bags")
	}
}

func tradeFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := worldFixture(t)
	for i, player := range players[:2] {
		next := player.character.Clone()
		next.Gold = uint32((i + 1) * 100)
		next.Bag = game.Inventory{}
		next.Bag[0] = game.Item{ID: 32176, Count: byte((i + 1) * 10), Damage: byte(i + 2)}
		next.Bag[0].Metadata[6] = byte(i + 7)
		if err := s.commit(context.Background(), player, next); err != nil {
			t.Fatal(err)
		}
		if err := s.dispatch(context.Background(), player, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, wire := range wires {
		wire.Reset()
	}
	return s, players, wires
}
func tradeDo(t *testing.T, s *Server, c *Session, p []byte) {
	t.Helper()
	if err := s.dispatch(context.Background(), c, p); err != nil {
		t.Fatal(err)
	}
}

// tradeOffer is the Confirm button's AC25:2: gold, then bag slots.
func tradeOffer(gold uint32, slots ...byte) []byte {
	return protocol.Builder{25, 2}.U32(gold).Bytes(slots)
}

// openTrade requests a trade, which aLogin opens on both sides at once.
func openTrade(t *testing.T, s *Server, players []*Session, wires []*captureConn) {
	t.Helper()
	a, b := players[0], players[1]
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(b.character.ID))
	if a.trade == nil || a.trade != b.trade {
		t.Fatal("trade did not open")
	}
	if !contains(wires[0].packets(t), protocol.Builder{25, 1}.U32(b.character.ID)) || !contains(wires[1].packets(t), protocol.Builder{25, 1}.U32(a.character.ID)) {
		t.Fatal("native open packets missing")
	}
	for _, wire := range wires {
		wire.Reset()
	}
}
func tradeStored(t *testing.T, s *Server, players []*Session) [2]game.Character {
	t.Helper()
	var result [2]game.Character
	for i, c := range players[:2] {
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || len(chars) != 1 {
			t.Fatal(err)
		}
		result[i] = chars[0]
	}
	return result
}

func TestTradeNativeOfferAndAtomicCompletion(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	a, b := players[0], players[1]
	tradeDo(t, s, a, tradeOffer(0, 1))
	// FUN_003c704c: gold, then ID, count, damage, metadata 2..7, 0..1, 8..25.
	golden := []byte{25, 3, 0, 0, 0, 0, 0xb0, 0x7d, 10, 2, 0, 0, 0, 0, 7, 0, 0, 0}
	golden = append(golden, make([]byte, 18)...)
	if p := wires[1].packets(t); len(p) != 1 || !bytes.Equal(p[0], golden) {
		t.Fatalf("offer wire %v", p)
	}
	tradeDo(t, s, b, tradeOffer(50))
	if p := wires[0].packets(t); len(p) != 1 || !bytes.Equal(p[0], []byte{25, 3, 50, 0, 0, 0}) {
		t.Fatalf("gold offer wire %v", p)
	}
	wires[0].Reset()
	tradeDo(t, s, a, []byte{25, 3, 1})
	if wires[0].Len() != 0 || a.character.Gold != 100 || a.character.Bag[0].Count != 10 {
		t.Fatal("first acceptance transferred assets")
	}
	tradeDo(t, s, b, []byte{25, 3, 1})
	if a.trade != nil || b.trade != nil || a.character.Gold != 150 || b.character.Gold != 150 || !a.character.Bag[0].Empty() || b.character.Bag[1].Count != 10 || b.character.Bag[1].Damage != 2 || b.character.Bag[1].Metadata[6] != 7 {
		t.Fatal("trade result incorrect")
	}
	for _, wire := range wires[:2] {
		if !contains(wire.packets(t), []byte{25, 2, 4}) {
			t.Fatal("completion missing")
		}
	}
	if wires[2].Len() != 0 {
		t.Fatal("trade leaked to another map")
	}
	stored := tradeStored(t, s, players)
	if stored[0].Gold != 150 || stored[1].Gold != 150 || !stored[0].Bag[0].Empty() || stored[1].Bag[1].Metadata[6] != 7 {
		t.Fatal("pair result not durable")
	}
	for _, wire := range wires {
		wire.Reset()
	}
	// The client answers 25/2 by closing its form, which sends a cancel.
	tradeDo(t, s, a, []byte{25, 3, 2})
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, b, []byte{25, 3, 1})
	if wires[0].Len() != 0 || wires[1].Len() != 0 || a.character.Gold+b.character.Gold != 300 {
		t.Fatal("completion replayed")
	}
}

func TestTradeAcceptNeedsBothLockedOffers(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	a, b := players[0], players[1]
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, a, tradeOffer(20, 1))
	tradeDo(t, s, a, []byte{25, 3, 1})
	if a.trade == nil || a.trade.accepted != [2]bool{} {
		t.Fatal("acceptance before both offers counted")
	}
	tradeDo(t, s, a, tradeOffer(90)) // A locked offer does not change.
	if a.trade.offers[0].Gold != 20 || len(a.trade.offers[0].Items) != 1 {
		t.Fatal("locked offer replaced")
	}
	tradeDo(t, s, b, tradeOffer(0))
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, b, []byte{25, 3, 1})
	if a.character.Gold != 80 || b.character.Gold != 220 || !a.character.Bag[0].Empty() || b.character.Bag[1].Count != 10 {
		t.Fatal("locked offers not exchanged")
	}
}

func TestTradeRefusesMalformedAndUnavailable(t *testing.T) {
	s, players, wires := tradeFixture(t)
	a, b := players[0], players[1]
	for _, p := range [][]byte{{25}, {25, 1, 0}, {25, 2, 0, 0, 0}, {25, 3}, {25, 3, 1, 1}, {25, 3, 9}, {25, 4, 0, 0, 0, 0}, append(tradeOffer(0), make([]byte, 21)...)} {
		if err := s.dispatch(context.Background(), a, p); err == nil {
			t.Fatal("malformed trade accepted", p)
		}
	}
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(a.character.ID))
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(players[2].character.ID))
	if a.trade != nil || !contains(wires[0].packets(t), []byte{25, 2, 1}) {
		t.Fatal("trade with an absent player accepted")
	}
	tradeDo(t, s, players[2], []byte{12, 1})
	wires[2].Reset()
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(players[2].character.ID))
	if a.trade != nil || wires[2].Len() != 0 || !contains(wires[0].packets(t), []byte{25, 2, 5}) {
		t.Fatal("self/cross-map trade accepted")
	}
	openTrade(t, s, players, wires)
	tradeDo(t, s, a, protocol.Builder{25, 1}.U32(b.character.ID))
	if !contains(wires[0].packets(t), []byte{25, 2, 11}) {
		t.Fatal("second trade not refused")
	}
	for _, p := range [][]byte{tradeOffer(0, 0), tradeOffer(0, 51), tradeOffer(0, 2, 2)} {
		if err := s.dispatch(context.Background(), a, p); err == nil {
			t.Fatal("bad slots accepted", p)
		}
	}
	tradeDo(t, s, a, tradeOffer(0, 2)) // An empty slot ends the trade.
	if a.trade != nil || b.trade != nil || !contains(wires[0].packets(t), []byte{25, 2, 8}) || !contains(wires[1].packets(t), []byte{25, 2, 9}) {
		t.Fatal("empty slot offer kept trade")
	}
	openTrade(t, s, players, wires)
	tradeDo(t, s, a, tradeOffer(101))
	if a.trade != nil || !contains(wires[0].packets(t), []byte{25, 2, 6}) || !contains(wires[1].packets(t), []byte{25, 2, 9}) {
		t.Fatal("unowned gold offer kept trade")
	}
}

func TestTradeCancellationReleasesInventories(t *testing.T) {
	for _, action := range []string{"cancel", "move", "disconnect", "mutation"} {
		t.Run(action, func(t *testing.T) {
			s, players, wires := tradeFixture(t)
			openTrade(t, s, players, wires)
			a, b := players[0], players[1]
			tradeDo(t, s, a, tradeOffer(0, 1))
			for _, wire := range wires {
				wire.Reset()
			}
			tradeDo(t, s, a, []byte{23, 124, 1, 3, 0}) // Reserved inventory cannot be destroyed.
			if a.character.Bag[0].Count != 10 || a.trade == nil {
				t.Fatal("offered item changed while window open")
			}
			wires[0].Reset()
			partner, own := byte(9), []byte{25, 2, 3}
			switch action {
			case "cancel":
				tradeDo(t, s, b, []byte{25, 3, 2})
				a, b = b, a
				wires[0], wires[1] = wires[1], wires[0]
				own = nil
			case "move":
				tradeDo(t, s, a, protocol.Builder{6, 1, 0}.U16(a.character.X+1).U16(a.character.Y))
			case "disconnect":
				s.leaveWorld(a)
				partner, own = 1, nil
			case "mutation":
				next := a.character.Clone()
				next.Gold++
				if err := s.commit(context.Background(), a, next); err != nil {
					t.Fatal(err)
				}
			}
			if a.trade != nil || b.trade != nil || players[0].character.Bag[0].Count != 10 || players[1].character.Bag[0].Count != 20 {
				t.Fatal("cancel changed offered items or retained session")
			}
			if !contains(wires[1].packets(t), []byte{25, 2, partner}) {
				t.Fatal("partner not told of cancellation")
			}
			if own != nil && !contains(wires[0].packets(t), own) {
				t.Fatal("own window not closed")
			}
			if own == nil && contains(wires[0].packets(t), []byte{25, 2, 3}) {
				t.Fatal("closed window told again")
			}
		})
	}
}

func TestTradeSaveFailureAndStaleMetadataNeverPublishSuccess(t *testing.T) {
	for _, failure := range []string{"database", "metadata"} {
		t.Run(failure, func(t *testing.T) {
			s, players, wires := tradeFixture(t)
			openTrade(t, s, players, wires)
			a, b := players[0], players[1]
			tradeDo(t, s, a, tradeOffer(0, 1))
			tradeDo(t, s, b, tradeOffer(50))
			for _, wire := range wires {
				wire.Reset()
			}
			if failure == "database" {
				s.Store.Close()
			} else {
				if err := s.Store.UpdateCharacter(context.Background(), a.account.ID, a.character.ID, func(c *game.Character) error { c.Bag[0].Metadata[6]++; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			tradeDo(t, s, a, []byte{25, 3, 1})
			err := s.dispatch(context.Background(), b, []byte{25, 3, 1})
			if failure == "database" && err == nil {
				t.Fatal("failed DB save was ignored")
			}
			if a.character.Gold != 100 || b.character.Gold != 200 || a.character.Bag[0].Count != 10 || b.character.Bag[0].Count != 20 || a.trade != nil || b.trade != nil {
				t.Fatal("failed trade adopted assets")
			}
			for _, wire := range wires[:2] {
				if p := wire.packets(t); contains(p, []byte{25, 2, 4}) || !contains(p, []byte{25, 2, 8}) {
					t.Fatal("failed trade result", p)
				}
			}
		})
	}
}

func TestTradeConcurrentAcceptIsCommittedOnce(t *testing.T) {
	s, players, wires := tradeFixture(t)
	openTrade(t, s, players, wires)
	tradeDo(t, s, players[0], tradeOffer(0, 1))
	tradeDo(t, s, players[1], tradeOffer(50))
	for _, wire := range wires {
		wire.Reset()
	}
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for i := 0; i < 20; i++ {
		c := players[i%2]
		wg.Add(1)
		go func() { defer wg.Done(); errors <- s.dispatch(context.Background(), c, []byte{25, 3, 1}) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if players[0].character.Gold != 150 || players[1].character.Gold != 150 || !players[0].character.Bag[0].Empty() || players[1].character.Bag[1].Count != 10 {
		t.Fatal("concurrent acceptance duplicated assets")
	}
	for _, wire := range wires[:2] {
		count := 0
		for _, p := range wire.packets(t) {
			if bytes.Equal(p, []byte{25, 2, 4}) {
				count++
			}
		}
		if count != 1 {
			t.Fatal("completion count", count)
		}
	}
}

func TestTradeTransfersMountedVehicleAndDismountsOwner(t *testing.T) {
	s, players, wires := tradeFixture(t)
	a, b := players[0], players[1]
	s.Assets.Items[48016] = game.ItemDefinition{ID: 48016, Type: game.VehicleType}
	next := a.character.Clone()
	next.Bag[1] = game.Item{ID: 48016, Count: 1, Damage: 27}
	next.ActiveVehicle, next.VehicleSlot = 48016, 2
	if err := s.commit(context.Background(), a, next); err != nil {
		t.Fatal(err)
	}
	openTrade(t, s, players, wires)
	tradeDo(t, s, a, tradeOffer(0, 2))
	tradeDo(t, s, b, tradeOffer(0))
	tradeDo(t, s, a, []byte{25, 3, 1})
	tradeDo(t, s, b, []byte{25, 3, 1})
	if a.character.ActiveVehicle != 0 || a.character.VehicleSlot != 0 || !a.character.Bag[1].Empty() {
		t.Fatal("transferred vehicle retained by owner")
	}
	if b.character.Bag[1].ID != 48016 || b.character.Bag[1].Damage != 27 || b.character.ActiveVehicle != 0 {
		t.Fatal("recipient lost vehicle state or boarded automatically")
	}
	dismount := protocol.Builder{15, 11, 2}.U32(a.character.ID)
	for _, wire := range wires[:2] {
		if !contains(wire.packets(t), dismount) {
			t.Fatal("dismount not replicated")
		}
	}
	saved := tradeStored(t, s, players)
	if saved[0].ActiveVehicle != 0 || saved[1].Bag[1].Damage != 27 {
		t.Fatal("vehicle transfer not saved")
	}
}
