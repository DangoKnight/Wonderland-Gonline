package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func TestShopSale(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	s.Assets.SalePrices = map[uint16]assets.SalePrice{32176: {Price: 3}, 21001: {Price: 50, Flags: 16}}
	wires[0].Reset()
	sell := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, append([]byte{27, 2}, p...)); err != nil {
			t.Fatal(err)
		}
		return wires[0].packets(t)
	}
	if got := sell(1, 1); len(got) != 1 || !bytes.Equal(got[0], []byte{27, 2, saleRejected}) {
		t.Fatal("sold without an open shop", got)
	}
	if err := s.openService(c, 2); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); !bytes.Equal(got[0], []byte{27, 3}) {
		t.Fatal("props shop", got)
	}
	if got := sell(1, 0); !bytes.Equal(got[0], []byte{27, 2, saleRejected}) {
		t.Fatal("sold props in the equipment mode", got)
	}
	got := sell(1, 1)
	if len(got) != 3 || !bytes.Equal(got[0], []byte{23, 9, 1, 50}) || !bytes.Equal(got[1], protocol.Builder{26, 4}.U32(150)) || !bytes.Equal(got[2], []byte{27, 2, saleSuccess}) {
		t.Fatal("sale", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Gold != 150 || !chars[0].Bag[0].Empty() {
		t.Fatal("sale not persisted", err)
	}
	if got = sell(1, 1); !bytes.Equal(got[0], []byte{27, 2, saleUnavailable}) {
		t.Fatal("sold an empty slot", got)
	}
}

func TestStorageTransfers(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	do := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wires[0].packets(t)
	}
	// Deposit bag slot 1, then withdraw storage slot 1 into bag slot 7.
	got := do(30, 2, 1)
	if !bytes.Equal(got[0], []byte{23, 9, 1, 50}) || !bytes.Equal(got[1], []byte{30, 8}) || c.character.Storage[0] != (game.Item{ID: 32176, Count: 50}) || !c.character.Bag[0].Empty() {
		t.Fatal("deposit", got)
	}
	got = do(30, 5, 7, 1)
	if got[0][1] != 5 || got[0][2] != 7 || c.character.Bag[6].Count != 50 || !c.character.Storage[0].Empty() {
		t.Fatal("withdraw", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag[6].Count != 50 {
		t.Fatal("storage not persisted", err)
	}
	// A failed move still answers with the storage snapshot.
	if got = do(30, 1, 9); len(got) != 4 || !bytes.Equal(got[0], []byte{30, 8}) {
		t.Fatal("empty withdraw", got)
	}
}

func TestClinicRest(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.openService(c, 6); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); !bytes.Equal(got[0], protocol.Builder{31, 2}.U32(0xffffffff)) {
		t.Fatal("full HP still offered a rest", got)
	}
	c.character.HP = 10
	if err := s.openService(c, 6); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{31, 1}); err != nil {
		t.Fatal(err)
	}
	got := wires[0].packets(t)
	if len(got) != 18 || !bytes.Equal(got[17], []byte{31, 1, 0}) || c.character.HP != c.character.MaxHP {
		t.Fatal("rest", got)
	}
	if err := s.worldCommand(ctx, c, []byte{31, 1}); err != nil || wires[0].Len() != 0 {
		t.Fatal("rest confirmed twice", err)
	}
}

func TestMenuWarps(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	for _, id := range []uint16{11016, 11094} {
		s.Assets.Maps[id] = assets.Map{ID: id}
	}
	s.World = world.New(s.Assets)
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.worldCommand(ctx, c, []byte{5, 17, 3}); err != nil || c.character.Map != 11094 || c.carnieReturn == nil || c.carnieReturn.Map != 10017 {
		t.Fatal("carnie", c.character.Map, err)
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	// Carnie's exit returns to where the visit began.
	c.lastWarp = time.Time{}
	c.character.X, c.character.Y = 600, 600
	if err := s.worldCommand(ctx, c, []byte{20, 8, 1, 0}); err != nil || c.character.Map != 10017 || c.character.X != 1042 {
		t.Fatal("carnie exit", c.character.Map, err)
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	// Without a record point, the record warp goes to the starter beach.
	if err := s.worldCommand(ctx, c, []byte{5, 17, 2}); err != nil || c.character.Map != 11016 {
		t.Fatal("record point fallback", c.character.Map, err)
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{5, 7}); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], protocol.Builder{5, 8}.U32(c.character.ID).U8(0)) {
		t.Fatal("sprite refresh", got)
	}
}
