package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

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
