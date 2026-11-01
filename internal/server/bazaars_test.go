package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"wonderland-gonline/internal/assets"
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
