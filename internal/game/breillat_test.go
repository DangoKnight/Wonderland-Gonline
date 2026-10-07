package game

import (
	"errors"
	"testing"
)

func breillatOutfitItems() map[uint16]ItemDefinition {
	return map[uint16]ItemDefinition{
		21001: {ID: 21001, EquipSlot: 2}, 24001: {ID: 24001, EquipSlot: 5},
		22007: {ID: 22007, EquipSlot: 1}, 21009: {ID: 21009, EquipSlot: 2},
		10002: {ID: 10002, EquipSlot: 3}, 24009: {ID: 24009, EquipSlot: 5},
		23001: {ID: 23001, EquipSlot: 4}, 32176: {ID: 32176, Type: 23},
	}
}

func TestReplaceBreillatOutfit(t *testing.T) {
	items := breillatOutfitItems()
	original := Item{ID: 23001, Count: 1, Damage: 17}
	original.Metadata[ForgeMetadataOffset] = 5
	original.Metadata[0] = 42
	c := Character{Body: 1}
	c.Bag[0] = Item{ID: 21001, Count: 2}
	c.Bag[3] = Item{ID: 24001, Count: 1}
	c.Bag[7] = Item{ID: 32176, Count: 50, Locked: true}
	c.Equipment[1] = Item{ID: 21001, Count: 1}
	c.Equipment[3] = original
	c.Storage[0] = Item{ID: 21001, Count: 1}
	if err := c.ReplaceBreillatOutfit(items); err != nil {
		t.Fatal(err)
	}
	if c.Equipment.IDs() != [6]uint16{22007, 21009, 10002, 0, 24009, 0} {
		t.Fatal(c.Equipment)
	}
	if c.Bag[0] != original || !c.Bag[3].Empty() || c.Bag[7] != (Item{ID: 32176, Count: 50, Locked: true}) {
		t.Fatal(c.Bag)
	}
	if c.Storage[0].ID != 21001 {
		t.Fatal("storage changed")
	}
	for _, item := range c.Bag {
		if item.ID == 21001 || item.ID == 24001 {
			t.Fatal("standard item retained", item)
		}
	}
}

func TestReplaceBreillatOutfitFailuresAreAtomic(t *testing.T) {
	for _, scenario := range []string{"full", "locked bag", "locked worn", "missing definition", "invalid footprint"} {
		t.Run(scenario, func(t *testing.T) {
			items := breillatOutfitItems()
			c := Character{Body: 1}
			c.Equipment[3] = Item{ID: 23001, Count: 1}
			want := ErrInventoryFull
			switch scenario {
			case "full":
				for i := range c.Bag {
					c.Bag[i] = Item{ID: 32176, Count: 50}
				}
			case "locked bag":
				c.Bag[0] = Item{ID: 21001, Count: 1, Locked: true}
				want = ErrItemLocked
			case "locked worn":
				c.Equipment[3].Locked = true
				want = ErrItemLocked
			case "missing definition":
				delete(items, 10002)
				want = nil
			case "invalid footprint":
				d := items[23001]
				d.CellWidth = 6
				d.CellHeight = 1
				items[23001] = d
				want = nil
			}
			before := c.Clone()
			err := c.ReplaceBreillatOutfit(items)
			if err == nil || want != nil && !errors.Is(err, want) {
				t.Fatal(err)
			}
			if c.Bag != before.Bag || c.Equipment != before.Equipment {
				t.Fatal("failed replacement mutated items")
			}
		})
	}
}

func TestReplaceBreillatOutfitUsesFreedSpace(t *testing.T) {
	c := Character{Body: 1}
	for i := range c.Bag {
		c.Bag[i] = Item{ID: 32176, Count: 50}
	}
	c.Bag[0] = Item{ID: 24001, Count: 1}
	c.Equipment[3] = Item{ID: 23001, Count: 1}
	if err := c.ReplaceBreillatOutfit(breillatOutfitItems()); err != nil {
		t.Fatal(err)
	}
	if c.Bag[0].ID != 23001 {
		t.Fatal("did not reuse removed standard item's space")
	}
}
