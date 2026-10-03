package game

import "testing"

func TestRepairPaymentCannotConsumeTarget(t *testing.T) {
	for _, count := range []byte{1, 2} {
		c := Character{Gold: 500}
		c.Bag[0] = Item{ID: 48050, Count: count, Damage: 9, Metadata: [26]byte{3}}
		tool, ok := c.RepairBagItem(1)
		if !ok || c.Bag[0].Empty() || c.Bag[0].Damage != 0 || c.Bag[0].Metadata[0] != 3 {
			t.Fatal(c, tool)
		}
		if count == 1 && (tool != 0 || c.Gold != 0 || c.Bag[0].Count != 1) {
			t.Fatal("sole target wrench consumed", c)
		}
		if count == 2 && (tool != 1 || c.Gold != 500 || c.Bag[0].Count != 1) {
			t.Fatal("stack wrench payment", c)
		}
	}
	c := Character{}
	c.Bag[0] = Item{ID: 48050, Count: 1, Damage: 8}
	before := c
	if _, ok := c.RepairBagItem(1); ok || c.Bag != before.Bag {
		t.Fatal("sole wrench destroyed")
	}
	for _, slot := range []byte{0, 2, 51, 255} {
		if _, ok := c.RepairBagItem(slot); ok || c.Bag != before.Bag {
			t.Fatal("invalid repair", slot)
		}
	}
}
