package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestPotentialSkillLossPreview(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	c.Stats.Element, c.Stats.Potential, c.Stats.STR = 3, 7, 16
	c.Stats.CON, c.Stats.INT, c.Stats.WIS, c.Stats.AGI = 13, 13, 13, 13
	c.SkillState.Learned = map[uint16]skills.Progress{15101: {Grade: 1}, 11016: {Grade: 1}}
	c.Skills.Catalog.Definitions[15101] = skills.Definition{Skill: assets.Skill{Name: "Sword Awn Attack"}}
	names := c.potentialSkillsAtRisk()
	if len(names) != 1 || names[0] != "Sword Awn Attack" {
		t.Fatal("incorrect endangered skill list", names)
	}
	if len(c.SkillState.Learned) != 2 || c.Stats.STR != 16 {
		t.Fatal("preview mutated live skills or attributes")
	}
	c.InventoryState.Bag[0] = game.Item{ID: game.ItemPotentialPill, Count: 12}
	c.Inventory.Use(1, false)
	d := c.Inventory.UseDialog
	d.SelectTarget(0)
	inventoryClick(t, c, d.Confirm)
	if c.UI.Modal == d {
		t.Fatal("missing skill loss confirmation")
	}
	c.Frame()
	if dir := os.Getenv("POTENTIAL_SNAPSHOT_DIR"); dir != "" {
		savePNG(t, filepath.Join(dir, "skill-warning.png"), c)
	}
	c.UI.Modal.KeyDown(0x1b, 0)
	if c.UI.Modal != d {
		t.Fatal("warning cancellation did not return to Potential")
	}
	c.Stats.STR = 20
	if len(c.potentialSkillsAtRisk()) != 0 {
		t.Fatal("safe skill warned")
	}
	c.Stats.STR = 16
	// Full native snapshot: one retained grade-one record and one explicit
	// zero-grade removal, followed by the eight-byte footer (potential seven).
	p := baseStats(3, 181, 100, [5]uint16{13, 16, 13, 13, 13}, 1, 0, 181, 100)
	p = p[:62]
	c.SkillState.Catalog.Orders[94] = 11016
	c.SkillState.Catalog.Orders[95] = 15101
	p = append(p, 2, 0, 94, 0, 1, 0, 0, 0, 0, 95, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0)
	c.dispatch(p)
	if _, exists := c.SkillState.Learned[15101]; exists {
		t.Fatal("forgotten skill survived the server snapshot")
	}
	if len(c.potentialSkillsAtRisk()) != 0 {
		t.Fatal("forgotten skill still triggers protection warning")
	}
	c.SkillState.Learned[15101] = skills.Progress{}
	if len(c.potentialSkillsAtRisk()) != 0 {
		t.Fatal("zero-grade skill triggers protection warning")
	}
}

func TestPotentialNativeWindowAndReplies(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.Inventory.Show()
	c.Stats.Level, c.Stats.STR, c.Stats.CON, c.Stats.INT, c.Stats.WIS, c.Stats.AGI = 1, 5, 0, 1, 1, 1
	c.InventoryState.Bag[0] = game.Item{ID: game.ItemPotentialPill, Count: 50}
	c.InventoryState.Bag[4] = game.Item{ID: game.ItemPotentialPill, Count: 2}
	c.InventoryState.Pets[2] = inventory.UsePet{ID: 12032, Name: []byte("Robinson"), Stats: world.Stats{Level: 1, STR: 15, CON: 20, INT: 5, WIS: 8, AGI: 12}}
	c.inventoryPacket([]byte{23, 5})
	c.Inventory.Use(1, false)
	d := c.Inventory.UseDialog
	if d == nil || d.Width != 299 || d.Height != 520 || d.Image != c.Pics.Find("Form_Potential_1") || c.UI.Modal != d || d.Confirm.Visible {
		t.Fatal("native Potential form or initial selection")
	}
	capture := wire(t, c)
	snapshot := func(name string) {
		c.Frame()
		if dir := os.Getenv("POTENTIAL_SNAPSHOT_DIR"); dir != "" {
			savePNG(t, filepath.Join(dir, name+".png"), c)
		}
	}
	snapshot("unselected")
	inventoryClick(t, c, d.Targets[0])
	if !d.Confirm.Visible {
		t.Fatal("player selection did not enable confirm")
	}
	snapshot("player")
	inventoryClick(t, c, d.Targets[1]) // Compact roster row maps to actual pet slot 3.
	snapshot("pet")
	a := d.Cancel.Abs()
	c.Input.X, c.Input.Y, c.Input.Hovered = a.X+8, a.Y+8, nil
	snapshot("cancel-hover")
	a = d.Confirm.Abs()
	c.Input.X, c.Input.Y, c.Input.Hovered = a.X+8, a.Y+8, nil
	snapshot("confirm-hover")
	before := c.InventoryState.Bag
	inventoryClick(t, c, d.Confirm)
	packets := capture()
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{23, 126, 1, 3}) || !d.Visible || d.Confirm.Visible || c.InventoryState.Bag != before || c.InventoryState.Pets[2].Potential != 0 {
		t.Fatal("attempt is not gated by authoritative replies", packets)
	}
	d.Confirm.Click()
	if len(capture()) != 1 {
		t.Fatal("duplicate pending attempt")
	}
	c.dispatch([]byte{23, 9, 1, 1})
	c.dispatch([]byte{23, 213, 1, 3, 1})
	c.Frame()
	if !d.Visible || !d.Confirm.Visible || c.InventoryState.Pets[2].Potential != 1 || c.InventoryState.Bag[0].Count != 49 {
		t.Fatal("result did not update reusable native window")
	}
	snapshot("success")
	// Once the first stack is exhausted the native form selects another stack.
	c.dispatch([]byte{23, 9, 1, 49})
	inventoryClick(t, c, d.Confirm)
	packets = capture()
	if len(packets) != 2 || !bytes.Equal(packets[1], []byte{23, 126, 5, 3}) {
		t.Fatal("next stack request", packets)
	}
	c.dispatch([]byte{23, 213, 0, 3, 1})
	c.InventoryState.Pets[2].ID = 14156
	c.Frame()
	if d.Confirm.Visible {
		t.Fatal("replacement pet inherited selected target")
	}
	d.KeyDown(0x1b, 0)
	if d.Visible || c.UI.Modal != nil || !c.Inventory.Visible {
		t.Fatal("Escape did not return to inventory")
	}
}

type potentialPoseFixture map[int]bool

func (p potentialPoseFixture) FrameSize(action int) (int, int) {
	if p[action] {
		return 32, 64
	}
	return 0, 0
}

func TestPotentialPreviewsNeverUseStandingPose(t *testing.T) {
	for _, tt := range []struct {
		frames potentialPoseFixture
		want   int
		ok     bool
	}{
		{potentialPoseFixture{49: true, 11: true}, 49, true},
		{potentialPoseFixture{47: true, 11: true}, 47, true},
		{potentialPoseFixture{11: true}, 0, false},
	} {
		if action, ok := potentialSittingAction(tt.frames); action != tt.want || ok != tt.ok {
			t.Fatal("standing pose substituted for sitting", action, ok)
		}
	}
}
