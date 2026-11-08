package app

import (
	"testing"
	"time"
)

func TestInventoryPointAttentionAcknowledgement(t *testing.T) {
	c, _, _ := enterBattle(t)
	c.Stats.Points = 3
	c.inventoryPointTick()
	b := c.MainButtons.Buttons[mainInventoryButton]
	if !b.Animating || b.AnimImage < 0 || b.AnimCount < 2 {
		t.Fatal("missing native inventory attention animation")
	}
	left, top := b.Left, b.Top
	c.inventoryPointTick()
	if b.Left != left || b.Top != top {
		t.Fatal("animation origin accumulated")
	}
	c.Inventory.Show()
	c.inventoryPointTick()
	if b.Animating || b.Left != left-2 || b.Top != top-8 {
		t.Fatal("opening inventory did not acknowledge alert")
	}
	c.Inventory.Hide()
	c.inventoryPointTick()
	if b.Animating {
		t.Fatal("closing inventory rearmed acknowledged points")
	}
	c.Stats.Points = 6
	c.inventoryPointTick()
	if !b.Animating {
		t.Fatal("new points did not rearm alert")
	}
	c.Stats.Points = 0
	c.inventoryPointTick()
	if b.Animating {
		t.Fatal("spent points still highlighted")
	}
	point := c.Pics.Find("Btn_Point_1")
	w, h := c.Pics.Size(point)
	if point < 0 || w == 0 || h%3 != 0 {
		t.Fatal("missing native three-frame POINT artwork")
	}
}

func TestInventoryPointAttentionSlowerPace(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Stats.Points = 3
	c.inventoryPointTick()
	b := c.MainButtons.Buttons[mainInventoryButton]
	first := b.AnimFrame
	*now = now.Add(149 * time.Millisecond)
	b.Update(c.Input)
	if b.AnimFrame != first {
		t.Fatal("attention frame advanced before 150 ms")
	}
	*now = now.Add(time.Millisecond)
	b.Update(c.Input)
	if b.AnimFrame != first+1 {
		t.Fatal("attention frame did not advance at 150 ms")
	}
}
