package server

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestHotelRepeatedTransfersSameSession(t *testing.T) {
	s, players, _ := petFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	next.Pets = []game.Pet{{ID: 14000, Slot: 1, Name: "Guest", Level: 2, Amity: 80}}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	c.pets.register(14000)
	if err := s.openService(c, world.ServicePetHotel); err != nil {
		t.Fatal(err)
	}
	for i, packet := range [][]byte{{31, 3, 1}, {31, 2, 1}, {31, 3, 1}, {31, 2, 1}} {
		if err := s.worldCommand(ctx, c, packet); err != nil {
			t.Fatalf("transfer %d: %v", i, err)
		}
		stored, err := s.Store.Characters(ctx, c.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		wantTeam := i % 2
		if len(stored[0].Pets) != wantTeam || len(stored[0].HotelPets) != 1-wantTeam || len(c.character.Pets) != wantTeam || len(c.character.HotelPets) != 1-wantTeam {
			t.Fatalf("transfer %d: incorrect persisted/session roster", i)
		}
	}
}

func TestHotelTransferRejectsStalePetResources(t *testing.T) {
	s, players, wires := petFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	next.Pets = []game.Pet{{ID: 14000, Slot: 1, Name: "Guest", Level: 2, Amity: 80}}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	c.pets.register(14000)
	c.hotelMap = c.character.Map
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(x *game.Character) error {
		x.Pets[0].Amity = 70
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{31, 3, 1}); !errors.Is(err, errNPCResourceConflict) {
		t.Fatalf("stale pet transfer: %v", err)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored[0].Pets) != 1 || stored[0].Pets[0].Amity != 70 || len(stored[0].HotelPets) != 0 || len(c.character.Pets) != 1 || c.pets.slot(14000) != 1 || wires[0].Len() != 0 {
		t.Fatal("stale transfer changed ownership or published success")
	}
}

func TestNativeNPCSaleCapturedGoldReceiptOrder(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	c.saleMode, c.saleMap = 1, c.character.Map
	s.Assets.SalePrices = map[uint16]assets.SalePrice{c.character.Bag[0].ID: {Price: 2}}
	wires[0].Reset()
	if err := s.sellItems(context.Background(), c, []byte{27, 2, 1, 1}); err != nil {
		t.Fatal(err)
	}
	want := [][]byte{{23, 9, 1, 10}, {26, 1, 20, 0, 0, 0}, {27, 2, 0}}
	got := wires[0].packets(t)
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatal(got)
		}
	}
}
func TestNPCResourceMutationRejectsStaleInventory(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	c.saleMode, c.saleMap = 1, c.character.Map
	s.Assets.SalePrices = map[uint16]assets.SalePrice{c.character.Bag[0].ID: {Price: 2}}
	before := c.character.Clone()
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(x *game.Character) error { x.Bag[0] = game.Item{}; return nil }); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.sellItems(context.Background(), c, []byte{27, 2, 1, 1}); !errors.Is(err, errNPCResourceConflict) {
		t.Fatal(err)
	}
	if c.character.Bag != before.Bag || c.character.Gold != before.Gold || wires[0].Len() != 0 {
		t.Fatal("stale resources published")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !saved[0].Bag[0].Empty() || saved[0].Gold != before.Gold {
		t.Fatal("SQL resources overwritten", err)
	}
}
func TestNPCStorageRequiresOpenService(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	before := c.character.Clone()
	wires[0].Reset()
	if err := s.storageCommand(context.Background(), c, []byte{30, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag != before.Bag || wires[0].Len() != 0 {
		t.Fatal("remote storage mutation")
	}
	if err := s.openService(c, world.ServicePropsKeeper); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.storageCommand(context.Background(), c, []byte{30, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if !c.character.Bag[0].Empty() {
		t.Fatal("authorized transfer missing")
	}
}
func TestNativeShopPickerOwnsCartWithoutUnlockingOtherEvents(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	m := s.Assets.Maps[c.character.Map]
	// Native shop header: category 3, layout 1, trailing byte 0.
	// A hidden type-4 entry must not be authorized as stock.
	m.Interactive = []assets.Group{{ClickID: 6, Header: []byte{3, 1, 0}, Members: []byte{5, 176, 125, 2, 3, 6, 176, 125, 2, 4}}}
	s.Assets.Maps[c.character.Map] = m
	s.Assets.NPCBuyPrices = map[uint16]uint32{32176: 7}
	es := &eventSession{mapID: c.character.Map}
	c.event = es
	s.authorizeShopQuestion(c, es, 6)
	if !es.shopCart || c.view.ShopItems[5] != 32176 || len(c.view.ShopItems) != 1 {
		t.Fatal("missing owned native cart")
	}
	wires[0].Reset()
	if err := s.buyNPCItems(context.Background(), c, []byte{27, 1, 1, 5, 1}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 3 || !bytes.Equal(got[0], []byte{26, 2, 7, 0, 0, 0}) || got[1][0] != 23 || got[1][1] != 6 || !bytes.Equal(got[2], []byte{27, 1, 0}) {
		t.Fatal(got)
	}
	before := c.character.Gold
	wires[0].Reset()
	if err := s.buyNPCItems(context.Background(), c, []byte{27, 1, 1, 5, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.Gold != before || !contains(wires[0].packets(t), []byte{27, 1, 1}) {
		t.Fatal("unrelated event accepted cart")
	}
	wires[0].Reset()
	if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || !contains(wires[0].packets(t), []byte{20, 8}) || len(c.view.ShopItems) != 0 {
		t.Fatal("native picker close did not release interaction")
	}
	if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if wires[0].Len() != 0 {
		t.Fatal("duplicate picker close produced a resume loop")
	}
}

func TestNPCResourcesPreserveBufferedMovementAndRecovery(t *testing.T) {
	s, players, _ := tradeFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	next.HP, next.SP = 10, 5
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	c.character.X += 20
	c.character.HP += 3
	c.character.SP += 2
	c.saleMode, c.saleMap = 1, c.character.Map
	s.Assets.SalePrices = map[uint16]assets.SalePrice{c.character.Bag[0].ID: {Price: 2}}
	if err := s.sellItems(ctx, c, []byte{27, 2, 1, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.X != next.X+20 || c.character.HP != 13 || c.character.SP != 7 {
		t.Fatal("pending state replaced")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].X != next.X || saved[0].HP != 10 || saved[0].SP != 5 {
		t.Fatal("unrelated state checkpointed", err)
	}
	s.autosaveCharacters(ctx)
	saved, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].X != next.X+20 || saved[0].HP != 13 || saved[0].SP != 7 {
		t.Fatal("checkpoint lost service state", err)
	}
}
