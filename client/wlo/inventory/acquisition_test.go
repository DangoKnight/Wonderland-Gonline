package inventory

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestCapturedLegacyInventoryAcquisition(t *testing.T) {
	s := State{Items: map[uint16]assets.NativeItem{46062: {Definition: game.ItemDefinition{ID: 46062, Type: 23}}}}
	// Independent live Bee Spine grant, observed identically for loot and pickup.
	p := []byte{23, 6, 238, 179, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	for i := 0; i < 2; i++ {
		if handled, valid := s.Apply(p); !handled || !valid {
			t.Fatal("legacy gain rejected")
		}
	}
	if s.Bag[0].ID != 46062 || s.Bag[0].Count != 2 {
		t.Fatal("native auto-stack failed", s.Bag)
	}
	before := s.Bag
	if _, valid := s.Apply(p[:32]); valid || s.Bag != before {
		t.Fatal("malformed acquisition mutated inventory")
	}
}
