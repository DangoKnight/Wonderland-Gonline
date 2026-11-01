package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestNPCShopCatalogPurchaseAndAuthority(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	s.Assets.NPCBuyPrices = map[uint16]uint32{32176: 7}
	m := s.Assets.Maps[c.character.Map]
	m.Interactive = []assets.Group{{Header: []byte{3}, Members: []byte{1, 0xb0, 0x7d, 0, 0}}}
	s.Assets.Maps[c.character.Map] = m
	if err := s.openService(c, world.ServicePropsShop); err != nil {
		t.Fatal(err)
	}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{35, 12, 133, 251, 1, 0}) {
		t.Fatal(got)
	}
	// The cart's byte slot resolves through the current map's authored catalog.
	tradeDo(t, s, c, []byte{27, 1, 1, 1, 3})
	if c.character.Gold != 79 || c.character.Bag[1].ID != 32176 || c.character.Bag[1].Count != 3 {
		t.Fatal("NPC purchase")
	}
	stored := tradeStored(t, s, players)
	if stored[0].Gold != 79 || stored[0].Bag != c.character.Bag {
		t.Fatal("purchase not durable")
	}
	before := c.character.Clone()
	wires[0].Reset()
	tradeDo(t, s, c, protocol.Builder{27, 1}.U16(21001).U8(1))
	if c.character.Gold != before.Gold || c.character.Bag != before.Bag || !contains(wires[0].packets(t), []byte{27, 1, 1}) {
		t.Fatal("unlisted item bought")
	}
	// A stale rich session cannot spend a balance already removed from SQL.
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(c *game.Character) error { c.Gold = 0; return nil }); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{27, 1, 1, 1, 1})
	stored = tradeStored(t, s, players)
	if stored[0].Gold != 0 || stored[0].Bag != before.Bag || !contains(wires[0].packets(t), []byte{27, 1, 2}) {
		t.Fatal("purchase trusted cached balance")
	}
}
