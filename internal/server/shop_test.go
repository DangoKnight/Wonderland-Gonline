package server

import (
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestParseShopCartNativeForms(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want []shopCartEntry
	}{
		{"counted pairs", []byte{2, 5, 2, 6, 3}, []shopCartEntry{{5, 2}, {6, 3}}},
		{"counted triples", []byte{1, 0x2c, 0x7d, 2}, []shopCartEntry{{32044, 2}}},
		{"plain triples", []byte{0x2c, 0x7d, 2}, []shopCartEntry{{32044, 2}}},
		{"plain pairs", []byte{5, 2, 6, 3}, []shopCartEntry{{5, 2}, {6, 3}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseShopCart(tc.data)
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

func TestResolveCatalogItemUsesEveInteractive(t *testing.T) {
	s := &Server{Assets: &assets.Catalog{Maps: map[uint16]assets.Map{
		12000: {Interactive: []assets.Group{{
			Header:  []byte{3, 0, 0},
			Members: []byte{1, 0x2c, 0x7d, 0, 0, 2, 0x11, 0x7d, 0, 0},
		}}},
	}}}
	c := &Session{character: &game.Character{Map: 12000}}
	if got := s.resolveCatalogItem(c, npcCatalogProps, 1); got != 32044 {
		t.Fatalf("slot 1 = %d, want 32044", got)
	}
	if got := s.resolveCatalogItem(c, npcCatalogProps, 2); got != 32017 {
		t.Fatalf("slot 2 = %d, want 32017", got)
	}
}

func TestResolveCatalogItemFallback(t *testing.T) {
	s := &Server{Assets: &assets.Catalog{Maps: map[uint16]assets.Map{}}}
	c := &Session{character: &game.Character{Map: 12000}}
	if got := s.resolveCatalogItem(c, npcCatalogWeapon, 1); got != 17064 {
		t.Fatalf("weapon fallback = %d, want 17064", got)
	}
	if got := s.resolveCatalogItem(c, npcCatalogProps, 2); got != 32002 {
		t.Fatalf("props fallback = %d, want 32002", got)
	}
	if got := s.resolveCatalogItem(c, npcCatalogProps, 20001); got != 20001 {
		t.Fatalf("explicit item = %d, want 20001", got)
	}
}
