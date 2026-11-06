package app

import "testing"

func TestFullItemNames(t *testing.T) {
	c := testClient(t)
	for id, want := range map[uint16]string{10034: "Pure Iron Knife", 10036: "Copper Falchion", 10039: "Pure Iron Falchion"} {
		if got := c.InventoryState.Items[id].Definition.Name; got != want {
			t.Fatal(id, got, want)
		}
	}
}
