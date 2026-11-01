package server

import (
	"context"
	"errors"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

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
