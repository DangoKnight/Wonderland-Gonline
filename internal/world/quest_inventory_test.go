package world

import (
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestQuestInventoryNativeCapacity(t *testing.T) {
	w := New(&assets.Catalog{Items: map[uint16]game.ItemDefinition{
		48008: {ID: 48008, CellWidth: 3, CellHeight: 2},
		30025: {ID: 30025, Type: 23},
		1:     {ID: 1},
	}})
	// Native 12159:3 vehicle reward and its paired inventory-full branch.
	can := [21]byte{15, 2, 0, 136, 187}
	cannot := [21]byte{15, 3, 0, 136, 187}
	c := &game.Character{}
	check := func(want bool) {
		t.Helper()
		before := c.Bag
		if w.condition(c, nil, 12159, nil, can) != want || w.condition(c, nil, 12159, nil, cannot) == want || c.Bag != before {
			t.Fatal("capacity predicate or non-mutating check failed", want)
		}
	}
	check(true)
	// More than six free cells, but no contiguous 3x2 rectangle.
	for i := range c.Bag {
		if i%game.BagColumns == 2 {
			c.Bag[i] = game.Item{ID: 1, Count: 1}
		}
	}
	check(false)
	c.Bag = game.Inventory{}
	c.Bag[0] = game.Item{ID: 48008, Count: 1}
	// One 3x2 raft occupies six cells, not just its anchor.
	free := [21]byte{15, 1, 0, 0, 0, 0, 0, 5, 44}
	if !w.condition(c, nil, 12159, nil, free) {
		t.Fatal("covered cells counted as available")
	}
	for i := range c.Bag {
		c.Bag[i] = game.Item{ID: 1, Count: 1}
	}
	c.Bag[0] = game.Item{ID: 30025, Count: 1}
	can[3], can[4] = 73, 117
	if !w.condition(c, nil, 60006, nil, can) {
		t.Fatal("existing compatible stack ignored")
	}
	c.Bag[0].Count = game.MaxItemStack
	if w.condition(c, nil, 60006, nil, can) {
		t.Fatal("full bag and stack authorized reward")
	}
	can[3], can[4] = 255, 255
	if w.condition(c, nil, 60006, nil, can) {
		t.Fatal("missing item authorized")
	}
}
