package game

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestItemReservationsProtectInventoryAndRemainTransient(t *testing.T) {
	item := Item{ID: 32176, Count: 3, Locked: true}
	bag := Inventory{item, {ID: 32176, Count: 2}}
	before := bag
	if err := bag.Remove(1, 1); !errors.Is(err, ErrItemLocked) {
		t.Fatal(err)
	}
	if _, err := bag.Move(2, 1, 1, 50); !errors.Is(err, ErrItemLocked) {
		t.Fatal(err)
	}
	if _, err := bag.Compound(1, 2, 10); !errors.Is(err, ErrItemLocked) {
		t.Fatal(err)
	}
	if bag != before {
		t.Fatal("failed operation changed inventory")
	}
	adds, err := bag.Grant(Item{ID: 32176}, 1, 50)
	if err != nil || len(adds) != 1 || adds[0].Slot != 2 || bag[0] != item {
		t.Fatal(adds, err)
	}
	c := Character{Bag: bag}
	c.Pets = []Pet{{ID: 123, Slot: 1, Equipment: Equipment{item}}}
	next := c.Clone()
	next.ClearItemLocks()
	if err = PreserveItemLocks(c, &next); err != nil || !next.Bag[0].Locked || !next.Pets[0].Equipment[0].Locked {
		t.Fatal(err)
	}
	next.Pets = nil
	if err = PreserveItemLocks(c, &next); !errors.Is(err, ErrItemLocked) {
		t.Fatal("reserved pet equipment disappeared", err)
	}
	raw, err := json.Marshal(c)
	var restored Character
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &restored); err != nil || restored.Bag[0].Locked || restored.Pets[0].Equipment[0].Locked {
		t.Fatal("reservation serialized", err)
	}
}

func TestRepairSkipsReservedItemAndTool(t *testing.T) {
	c := Character{Gold: 1000, Bag: Inventory{{ID: 10, Count: 1, Damage: 3, Locked: true}, {ID: RepairWrenchItemID, Count: 1, Locked: true}}}
	if _, ok := c.RepairBagItem(1); ok || c.Gold != 1000 {
		t.Fatal("repaired reserved item")
	}
	c.Bag[0].Locked = false
	if slot, ok := c.RepairBagItem(1); !ok || slot != 0 || c.Gold != 500 || c.Bag[1].Count != 1 {
		t.Fatal("reserved tool used")
	}
}
