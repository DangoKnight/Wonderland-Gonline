package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/world"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestParseShopCartNativeForms(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []npcShopCartEntry
	}{
		{"counted pairs", []byte{2, 5, 2, 6, 3}, []npcShopCartEntry{{5, 2}, {6, 3}}},
		{"counted triples", []byte{1, 0x2c, 0x7d, 2}, []npcShopCartEntry{{32044, 2}}},
		{"plain triples", []byte{0x2c, 0x7d, 2}, []npcShopCartEntry{{32044, 2}}},
		{"plain pairs", []byte{5, 2, 6, 3}, []npcShopCartEntry{{5, 2}, {6, 3}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeNPCShopCart(tc.data)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len=%d want %d: %#v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("entry %d=%#v want %#v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNPCShopCatalogUsesEveInteractiveSlots(t *testing.T) {
	s := &Server{Assets: &assets.Catalog{Maps: map[uint16]assets.Map{
		12000: {Interactive: []assets.Group{{Header: []byte{3, 0, 0}, Members: []byte{1, 0x2c, 0x7d, 0, 0, 2, 0x11, 0x7d, 0, 0}}}},
	}}}
	c := &Session{conn: &captureConn{}, character: &game.Character{Map: 12000}, view: world.NewView()}
	if err := s.openNPCShop(c, world.ServicePropsShop); err != nil {
		t.Fatal(err)
	}
	if c.view.ShopItems[1] != 32044 || c.view.ShopItems[2] != 32017 {
		t.Fatal(c.view.ShopItems)
	}
}

func TestNPCShopFallbackCatalogDoesNotAuthorizeUnlistedIDs(t *testing.T) {
	s, players, _ := tradeFixture(t)
	c := players[0]
	s.Assets.NPCShopDefaults = map[uint16][]uint16{world.ServiceWeaponShop: {17064}, world.ServicePropsShop: {32011, 32002}}
	if err := s.openService(c, world.ServiceWeaponShop); err != nil {
		t.Fatal(err)
	}
	if c.view.ShopItems[1] != 17064 {
		t.Fatal(c.view.ShopItems)
	}
	if err := s.openService(c, world.ServicePropsShop); err != nil {
		t.Fatal(err)
	}
	if c.view.ShopItems[2] != 32002 {
		t.Fatal(c.view.ShopItems)
	}
	s.Assets.Items[20001] = game.ItemDefinition{ID: 20001}
	before := c.character.Clone()
	if err := s.worldCommand(context.Background(), c, []byte{27, 1, 0x21, 0x4e, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.Gold != before.Gold || c.character.Bag != before.Bag {
		t.Fatal("unlisted explicit ID bypassed the active catalog")
	}
}
