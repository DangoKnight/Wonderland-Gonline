package app

import (
	"fmt"
	"testing"
)

func TestMallThunderSpearPreview(t *testing.T) {
	c, _, _ := enteredClient(t)
	it := c.items[12101]
	if it.Definition.EquipSlot != 3 {
		t.Fatal("Thunder Spear is not a hand weapon")
	}
	for i, id := range it.Sprites {
		if id == 0 {
			t.Fatal("missing body weapon sprite")
		}
		if _, err := c.lib.Image(fmt.Sprintf("00%dw", i+2), int(id), 17, 0); err != nil {
			t.Fatalf("body %d sprite %d: %v", i+1, id, err)
		}
	}
}
