package game

import (
	"errors"
	"testing"
)

func gridItems() map[uint16]ItemDefinition {
	return map[uint16]ItemDefinition{1: {ID: 1}, 2: {ID: 2}, 48016: {ID: 48016, CellWidth: 4, CellHeight: 3}, 3: {ID: 3, CellWidth: 2, CellHeight: 1}}
}

func TestMultiSlotGrantUsesContiguousFootprint(t *testing.T) {
	items := gridItems()
	var bag Inventory
	adds, err := bag.Grant(Item{ID: 48016}, 1, 1, items)
	if err != nil || len(adds) != 1 || adds[0] != (Addition{1, 1}) {
		t.Fatal(adds, err)
	}
	cells, err := bag.Occupancy(items)
	if err != nil {
		t.Fatal(err)
	}
	var want [BagSize]byte
	for _, index := range []int{0, 1, 2, 3, 5, 6, 7, 8, 10, 11, 12, 13} {
		want[index] = 1
	}
	if cells != want {
		t.Fatalf("raft footprint: %v", cells)
	}
	adds, err = bag.Grant(Item{ID: 1}, 1, 1, items)
	if err != nil || adds[0].Slot != 5 {
		t.Fatal("covered cells reused", adds, err)
	}
	if len(bag.Packet(23, 5)) != 2+2*31 {
		t.Fatal("covered cells serialized as duplicate items")
	}
	if err := bag.Remove(1, 1); err != nil {
		t.Fatal(err)
	}
	if !bag.CanPlace(1, Item{ID: 48016, Count: 1}, items) {
		t.Fatal("removal failed to release footprint")
	}
}

func TestMultiSlotFragmentationAndQuantityRollback(t *testing.T) {
	items := gridItems()
	var bag Inventory
	// Forty empty cells, but every row has a blocker in its middle column.
	for i := 2; i < BagSize; i += BagColumns {
		bag[i] = Item{ID: 1, Count: 1}
	}
	before := bag
	if adds, err := bag.Grant(Item{ID: 48016}, 1, 1, items); !errors.Is(err, ErrInventoryFull) || len(adds) != 0 || bag != before {
		t.Fatal("fragmented space accepted", adds, err)
	}
	// Only one 4x3 rectangle fits: two rafts must not partially deliver.
	for i := range bag {
		bag[i] = Item{ID: 1, Count: 1}
	}
	for _, index := range []int{0, 1, 2, 3, 5, 6, 7, 8, 10, 11, 12, 13} {
		bag[index] = Item{}
	}
	before = bag
	if adds, err := bag.Grant(Item{ID: 48016}, 2, 1, items); !errors.Is(err, ErrInventoryFull) || len(adds) != 0 || bag != before {
		t.Fatal("partial multi-slot delivery", adds, err)
	}
	bag[0].Locked = true
	if _, err := bag.Grant(Item{ID: 48016}, 1, 1, items); !errors.Is(err, ErrInventoryFull) {
		t.Fatal("reserved cell used", err)
	}
}

func TestMultiSlotMoveEdgesOverlapAndSplit(t *testing.T) {
	items := gridItems()
	var bag Inventory
	bag[0] = Item{ID: 48016, Count: 1}
	before := bag
	for _, slot := range []byte{5, 46} {
		if _, err := bag.Move(1, slot, 1, 1, items); !errors.Is(err, ErrInventoryFull) || bag != before {
			t.Fatal("edge wrap accepted", slot, err)
		}
	}
	if moved, err := bag.Move(1, 6, 1, 1, items); err != nil || moved != 1 || !bag[0].Empty() || bag[5].ID != 48016 {
		t.Fatal("self-overlapping whole move rejected", moved, err)
	}
	bag = Inventory{}
	bag[0] = Item{ID: 3, Count: 2}
	before = bag
	if _, err := bag.Move(1, 2, 1, MaxItemStack, items); !errors.Is(err, ErrInventoryFull) || bag != before {
		t.Fatal("split overlapped retained stack", err)
	}
	if _, err := bag.Move(1, 3, 1, MaxItemStack, items); err != nil || bag[0].Count != 1 || bag[2].Count != 1 {
		t.Fatal("split into free footprint", err)
	}
}

func TestMultiSlotTransferAndEquipmentRollback(t *testing.T) {
	items := gridItems()
	var source, target Inventory
	source[0] = Item{ID: 48016, Count: 1, Metadata: [ItemMetadataBytes]byte{7}}
	for i := 2; i < BagSize; i += BagColumns {
		target[i] = Item{ID: 1, Count: 1}
	}
	beforeSource, beforeTarget := source, target
	if err := Transfer(&source, &target, 1, 1, 1, items); !errors.Is(err, ErrInventoryFull) || source != beforeSource || target != beforeTarget {
		t.Fatal("failed transfer lost item", err)
	}
	c := Character{Bag: source}
	c.Equipment[0] = Item{ID: 1, Count: 1}
	if err := c.Unwear(1, 2, items); !errors.Is(err, ErrInventoryFull) || c.Equipment[0].Empty() || c.Bag != source {
		t.Fatal("unequipped onto covered cell", err)
	}
	target = Inventory{}
	if err := Transfer(&source, &target, 1, 1, 1, items); err != nil || target[0] != beforeSource[0] || !source[0].Empty() {
		t.Fatal("transfer metadata/footprint", err)
	}
}

func TestMultiSlotCompoundAndHandInCapacity(t *testing.T) {
	items := gridItems()
	var bag Inventory
	for i := range bag {
		bag[i] = Item{ID: 1, Count: 1}
	}
	before := bag
	if _, err := bag.Compound(1, 2, 48016, items); !errors.Is(err, ErrInventoryFull) || bag != before {
		t.Fatal("compound consumed ingredients without footprint", err)
	}
	bag = Inventory{}
	bag[0] = Item{ID: 48016, Count: 1}
	limit := func(id uint16) (byte, bool) { _, ok := items[id]; return 1, ok }
	result, err := bag.ApplyQuestItems([]ItemChange{{48016, -1}, {48016, 1}}, limit, items)
	if err != nil || bag[0].ID != 48016 || len(result.Added) != 0 || len(result.Removed) != 0 {
		t.Fatal("hand-in failed to release/reuse footprint", result, err)
	}
}
