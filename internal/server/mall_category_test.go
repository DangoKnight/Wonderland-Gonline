package server

import (
	"bytes"
	"math"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestMallSaleWireMatchesChargedCost(t *testing.T) {
	s := &Server{Assets: &assets.Catalog{Mall: []assets.MallItem{{ID: 55086, Count: 1, Cost: 20, OriginalPrice: 25, Discount: 80, Badge: 1, CategoryID: 3, Order: 13}}, Items: map[uint16]game.ItemDefinition{55086: {}}}}
	// Independent raw AC75 catalog record: ID55086, one, base25, 80%, NEW,
	// Grocery, order13. Purchase cost remains twenty, not sixteen.
	want := []byte{75, 1, 1, 0, 46, 215, 1, 25, 0, 80, 1, 3, 13, 0}
	if got := s.mallCatalogPacket(false); !bytes.Equal(got, want) {
		t.Fatalf("%v != %v", got, want)
	}
	for _, e := range []assets.MallItem{{Cost: 20}, {Cost: 20, OriginalPrice: 25, Discount: 90}, {Cost: 13, OriginalPrice: 14, Discount: 90}} {
		price, discount := mallAdvertisedPrice(e)
		if int(math.RoundToEven(float64(price)*float64(discount)/100)) != e.Cost {
			t.Fatal("advertised price differs from charge", e)
		}
	}
}

// Native FUN_00240584 routes 1 to Weaponry, 2 to Armory, 3/4 to Grocery
// and 5 to Furniture. Hot is built from badges, not a sixth item category.
func TestMallNativeWireCategoryNames(t *testing.T) {
	for _, tt := range []struct {
		name string
		wire byte
	}{{"weaponry", 1}, {"1", 1}, {"armory", 2}, {"2", 2}, {"grocery", 3}, {"3", 3}, {"4", 4}, {"furniture", 5}, {"5", 5}, {"unknown", 3}} {
		if got := mallCategory(assets.MallItem{Category: tt.name}); got != tt.wire {
			t.Fatalf("%s: got %d want %d", tt.name, got, tt.wire)
		}
	}
	if got := mallCategory(assets.MallItem{Category: "weaponry", CategoryID: 4}); got != 4 {
		t.Fatal("explicit catalog category changed", got)
	}
}
