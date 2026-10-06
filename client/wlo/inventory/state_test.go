package inventory

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestInventoryNativeUpdates(t *testing.T) {
	s := State{Items: map[uint16]assets.NativeItem{100: {Definition: game.ItemDefinition{EquipSlot: 1}}}}
	// Independent native AC23/5 fixture: slot, ID, count, damage, 26 metadata bytes.
	packet := []byte{23, 5, 1, 100, 0, 1, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0}
	apply := func(p []byte) {
		t.Helper()
		if handled, valid := s.Apply(p); !handled || !valid {
			t.Fatalf("rejected %x", p)
		}
	}
	apply(packet)
	if s.Bag[0].Count != 1 || s.Bag[0].Forge() != 3 {
		t.Fatal(s.Bag[0])
	}
	apply([]byte{23, 17, 1, 1})
	if !s.Bag[0].Empty() || s.Equipment[0].ID != 100 {
		t.Fatal("wear")
	}
	apply([]byte{23, 16, 1, 2})
	if !s.Equipment[0].Empty() || s.Bag[1].Forge() != 3 {
		t.Fatal("unequip metadata")
	}
	packet[2] = 2
	apply(packet)
	if s.Bag[1].Count != 2 {
		t.Fatal("addition replaced stack")
	}
	apply([]byte{23, 10, 2, 1, 50})
	if s.Bag[1].Count != 1 || s.Bag[49].Count != 1 || s.Bag[49].Forge() != 3 {
		t.Fatal("move")
	}
	before := s.Bag
	bad := append(append([]byte(nil), packet...), packet[2:]...)
	bad[33] = 51
	if handled, valid := s.Apply(bad); !handled || valid || s.Bag != before {
		t.Fatal("malformed packet partly applied")
	}
	apply([]byte{23, 9, 50, 1})
	if !s.Bag[49].Empty() {
		t.Fatal("removal")
	}
	s.Reset(nil)
	if s.Bag != (game.Inventory{}) || s.Equipment != (game.Equipment{}) {
		t.Fatal("character reset leaked items")
	}
}

func TestInventoryEquipmentSnapshotAndSwap(t *testing.T) {
	s := State{Items: map[uint16]assets.NativeItem{100: {Definition: game.ItemDefinition{EquipSlot: 1}}, 101: {Definition: game.ItemDefinition{EquipSlot: 1}}}}
	p := []byte{23, 11, 100, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0}
	if _, valid := s.Apply(p); !valid {
		t.Fatal("snapshot")
	}
	if s.Equipment[0].Forge() != 4 {
		t.Fatal("snapshot forge")
	}
	s.Bag[0] = game.Item{ID: 101, Count: 1}
	if _, valid := s.Apply([]byte{23, 17, 1, 1}); !valid {
		t.Fatal("swap")
	}
	if s.Bag[0].ID != 100 || s.Bag[0].Forge() != 4 || s.Equipment[0].ID != 101 {
		t.Fatal("swap lost old gear")
	}
	if _, valid := s.Apply([]byte{23, 16, 1, 1}); valid {
		t.Fatal("unequip overwrote occupied slot")
	}
	if _, valid := s.Apply([]byte{23, 11}); !valid || !s.Equipment[0].Empty() {
		t.Fatal("empty snapshot")
	}
}
