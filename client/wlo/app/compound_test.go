package app

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestCompoundFlow(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	f := c.Compound
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 3}
	c.InventoryState.Bag[1] = game.Item{ID: 7001, Count: 1}
	c.InventoryState.Bag[2] = game.Item{ID: 32011, Count: 1}
	c.MainButtons.Buttons[mainCompoundButton].OnClick()
	if !f.Visible || f.Width != 299 || f.Height != 467 {
		t.Fatal("compound toolbar/layout")
	}
	// Drag the first bag anchor into the base ingredient; left-click a second.
	a, b := f.Slots[0].Abs(), f.Ingredients[0].Abs()
	f.Slots[0].LeftDown(0, a.X+8, a.Y+8)
	f.Slots[0].CapturedMove(0, b.X+8, b.Y+8)
	f.Slots[0].LeftUpOutside(0, b.X+8, b.Y+8)
	b = f.Slots[1].Abs()
	f.Slots[1].LeftDown(0, b.X+8, b.Y+8)
	f.Slots[1].LeftUp(0, b.X+8, b.Y+8)
	if f.Selection[0] != 1 || f.Selection[1] != 2 {
		t.Fatal("ingredient ordering", f.Selection)
	}
	if f.SelectIngredient(1, 2) {
		t.Fatal("duplicate bag anchor selected")
	}
	f.Synthesize()
	f.Synthesize()
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{23, 14, 2, 1, 2}) {
		t.Fatal("compound request/repeated click", got)
	}
	if c.InventoryState.Bag[0].Count != 3 || c.InventoryState.Bag[1].Count != 1 {
		t.Fatal("optimistic consumption")
	}
	c.dispatch([]byte{23, 9, 1, 1})
	c.dispatch([]byte{23, 9, 2, 1})
	result := append([]byte{23, 8, 2, 0x59, 0x1b, 1}, make([]byte, 28)...)
	c.dispatch(result)
	c.dispatch([]byte{23, 13, 0x59, 0x1b, 1, 2})
	if c.InventoryState.Bag[0].Count != 2 || c.InventoryState.Bag[1].ID != 7001 || c.InventoryState.Bag[1].Count != 1 {
		t.Fatal("confirmed result", c.InventoryState.Bag[:2])
	}
	for _, slot := range f.Selection {
		if slot != 0 {
			t.Fatal("result did not clear ingredients")
		}
	}
	*now = now.Add(7 * time.Second)
	f.Refresh()
	// Larger recipes are sent completely; no ingredient is silently ignored.
	f.Slots[0].DblClick()
	f.Slots[1].DblClick()
	f.Slots[2].DblClick()
	f.Synthesize()
	got = sent()
	if len(got) != 2 || !bytes.Equal(got[1], []byte{23, 14, 3, 1, 2, 3}) {
		t.Fatal("three-input recipe", got)
	}

	*now = now.Add(6 * time.Second)
	f.Paint()
	f.Ingredients[2].RightUp(0, 0, 0)
	if !f.SelectIngredient(3, 2) {
		t.Fatal("rejected request did not release UI")
	}
	if !c.CompoundKey(27, 0) || f.Visible {
		t.Fatal("escape")
	}
	if !c.CompoundKey('C', 4) || !f.Visible {
		t.Fatal("keyboard opening")
	}
	c.resetInventory(c.World.Player)
	if f.Visible {
		t.Fatal("character reset retained compound window")
	}
}

func TestCompoundSelectionSafety(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	f := c.Compound
	f.Show()
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 2, Locked: true}
	if f.SelectIngredient(1, 0) {
		t.Fatal("locked ingredient")
	}
	c.InventoryState.Bag[0].Locked = false
	c.World.Player.VehicleID, c.World.Player.VehicleSlot = 32011, 1
	if f.SelectIngredient(1, 0) {
		t.Fatal("active vehicle ingredient")
	}
	c.World.Player.VehicleID = 0
	definition := c.InventoryState.Items[32011]
	definition.Definition.Type = game.ItemTypeExotic
	c.InventoryState.Items[32011] = definition
	if f.SelectIngredient(1, 0) {
		t.Fatal("Exotic ingredient selected")
	}
	c.InventoryState.Items[32011] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32011, Type: 31}}
	if !f.SelectIngredient(1, 0) {
		t.Fatal("unreserved ingredient")
	}
	c.InventoryState.Bag[0] = game.Item{ID: 7001, Count: 2}
	f.Refresh()
	if f.Selection[0] != 0 {
		t.Fatal("replacement inventory item retained old selection")
	}
	modal := c.Settings
	c.UI.Modal = modal
	if f.SelectIngredient(1, 0) {
		t.Fatal("modal allowed ingredient selection")
	}
	c.UI.Modal = nil
	c.mapReady = false
	if f.SelectIngredient(1, 0) {
		t.Fatal("loading allowed ingredient selection")
	}
}

func TestCompoundReferenceSnapshot(t *testing.T) {
	out := os.Getenv("COMPOUND_SNAPSHOT")
	if out == "" {
		t.Skip("set COMPOUND_SNAPSHOT")
	}
	c, now, _ := enteredClient(t)
	c.mapReady = true
	c.InventoryState.Bag[0] = game.Item{ID: 32011, Count: 5}
	c.InventoryState.Bag[1] = game.Item{ID: 32012, Count: 50}
	c.inventoryPacket([]byte{23, 5})
	c.Compound.Show()
	c.Frame()
	savePNG(t, out, c)
	base := strings.TrimSuffix(out, ".png")
	c.SkillState.Learned[game.AlchemyPrimarySkill] = skills.Progress{Grade: 1}
	c.SkillState.Learned[game.AlchemyJuniorSkill] = skills.Progress{Grade: 1}
	c.Compound.Refresh()
	c.Compound.Junior.Click()
	c.Frame()
	savePNG(t, base+"-junior.png", c)
	c.SkillState.Learned[game.AlchemySuperiorSkill] = skills.Progress{Grade: 1}
	c.Compound.SelectTier(game.AlchemySuperior)
	c.Frame()
	savePNG(t, base+"-superior.png", c)
	c.Compound.TierButton.Click()
	at := c.Compound.TierButton.Abs()
	c.Input.X, c.Input.Y = at.X+10, at.Y+10
	c.Frame()
	savePNG(t, base+"-tiers.png", c)
	c.Compound.SelectTier(game.AlchemyJunior)
	c.Compound.SelectIngredient(1, 0)
	c.Compound.SelectIngredient(2, 1)
	c.Compound.Synthesize()
	c.InventoryState.Bag[2] = game.Item{ID: 32011, Count: 1}
	c.Compound.QueueResult(3)
	c.Compound.Result(32011, 1, 3)
	*now = now.Add(3300 * time.Millisecond)
	c.Frame()
	savePNG(t, base+"-flight.png", c)
}

func TestCompoundNativeTierCommand(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	f := c.Compound
	f.Show()
	c.SkillState.Learned[game.AlchemyPrimarySkill] = skills.Progress{Grade: 30}
	if got := f.AlchemyCommand(); got != 14 {
		t.Fatal("Primary command", got)
	}
	c.SkillState.Learned[game.AlchemyJuniorSkill] = skills.Progress{Grade: 1}
	f.Refresh()
	if !f.Junior.Visible || f.TierButton.Visible || f.AlchemyCommand() != 14 {
		t.Fatal("Junior checkbox/default")
	}
	f.Junior.Click()
	if got := f.AlchemyCommand(); got != 87 {
		t.Fatal("Junior command", got)
	}
	f.Junior.Click()
	if got := f.AlchemyCommand(); got != 14 {
		t.Fatal("unchecked command", got)
	}
	c.SkillState.Learned[game.AlchemySuperiorSkill] = skills.Progress{Grade: 1}
	f.Refresh()
	if f.Junior.Visible || !f.TierButton.Visible {
		t.Fatal("Superior selector")
	}
	f.TierButton.Click()
	if !f.TierOptions[2].Visible {
		t.Fatal("tier menu")
	}
	f.TierOptions[2].Click()
	if got := f.AlchemyCommand(); got != 101 || f.TierOptions[2].Visible {
		t.Fatal("Superior command/close", got)
	}
	f.Hide()
	f.Show()
	if f.AlchemyCommand() != 101 {
		t.Fatal("choice lost across reopening")
	}
	delete(c.SkillState.Learned, game.AlchemySuperiorSkill)
	f.Refresh()
	if f.AlchemyCommand() != 14 {
		t.Fatal("unlearned selection retained")
	}
	if f.SelectTier(game.AlchemySuperior) {
		t.Fatal("unlearned tier selected")
	}
}
