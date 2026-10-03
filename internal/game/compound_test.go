package game

import "testing"

func TestCompoundKeepsLeftoversAndMetadata(t *testing.T) {
	var bag Inventory
	bag[4] = Item{ID: 100, Count: 3, Damage: 12, Metadata: [26]byte{7, 8}}
	bag[8] = Item{ID: 200, Count: 1, Damage: 4}
	before := bag
	slot, err := bag.Compound(9, 5, 300)
	if err != nil || slot != 1 {
		t.Fatal(slot, err)
	}
	want := before[4]
	want.Count--
	if bag[4] != want || !bag[8].Empty() || bag[0] != (Item{ID: 300, Count: 1}) {
		t.Fatal(bag)
	}
	// Lowest ingredient slot is reused when it is free.
	bag = Inventory{}
	bag[4] = Item{ID: 100, Count: 1}
	bag[8] = Item{ID: 200, Count: 1}
	if slot, err = bag.Compound(9, 5, 300); err != nil || slot != 5 {
		t.Fatal(slot, err)
	}
}

func TestCompoundRollsBackInvalidAndFullBags(t *testing.T) {
	var bag Inventory
	for i := range bag {
		bag[i] = Item{ID: 100, Count: 2}
	}
	for _, slots := range [][2]byte{{1, 2}, {0, 2}, {1, 51}, {1, 1}} {
		before := bag
		if _, err := bag.Compound(slots[0], slots[1], 300); err == nil || bag != before {
			t.Fatal("failed compound changed bag", slots, err)
		}
	}
	bag[1].Count = 1
	if slot, err := bag.Compound(1, 2, 300); err != nil || slot != 2 || bag[0].Count != 1 {
		t.Fatal(slot, err)
	}
	before := bag
	if _, err := bag.Compound(1, 3, 0); err == nil || bag != before {
		t.Fatal("invalid result changed bag")
	}
	bag[0] = Item{}
	before = bag
	if _, err := bag.Compound(1, 3, 300); err == nil || bag != before {
		t.Fatal("missing ingredient changed bag")
	}
}
