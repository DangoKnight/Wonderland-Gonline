package app

import (
	"encoding/binary"
	"os"
	"testing"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/game"
)

func skillSnapshot(records ...[]byte) []byte {
	p := baseStats(3, 181, 100, [5]uint16{1, 1, 1, 1, 1}, 1, 0, 181, 100)
	p = p[:64]
	binary.LittleEndian.PutUint16(p[62:], uint16(len(records)))
	for _, r := range records {
		p = append(p, r...)
	}
	return append(p, make([]byte, 8)...)
}
func TestSkillsNativeWindowTabsAndTree(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	// Avatar 4/head 5 has Fire Dance; AC5:3 identifies it through stunt alias 15003.
	c.World.Player.Body, c.World.Player.Head = 4, 5
	sent := wire(t, c)
	c.dispatch(skillSnapshot([]byte{188, 0, 1, 0, 0, 0, 0}, []byte{94, 0, 1, 0, 0, 0, 0}, []byte{113, 0, 1, 0, 0, 0, 0}, []byte{130, 0, 1, 0, 0, 0, 0}))
	c.MainButtons.Buttons[2].OnClick()
	f := c.Skills
	if !f.Visible || f.Image != c.Pics.Find("Form_Skill_2") || len(f.List.Items) != 3 || string(f.List.Items[2]) != "Flame Attack" {
		t.Fatal("physical window", f.List.Items)
	}
	for _, b := range append(f.Tabs[:], f.Previous, f.Next, f.Close, f.Up, f.Down) {
		if b.Image < 0 {
			t.Fatal("missing skill control", b.Tag)
		}
	}
	if f.Scroll.Image < 0 || f.Scroll.ThumbImg < 0 {
		t.Fatal("missing scroll assets")
	}
	for _, ic := range f.List.Icons {
		if ic == "-1" {
			t.Fatal("missing icon")
		}
	}
	f.Left, f.Top = 329, 60
	hover := func(row int) { a := f.List.Abs(); c.Input.X, c.Input.Y = a.X+75, a.Y+row*26+10; c.Frame(); c.Frame() }
	capture := func(name string) {
		if out := os.Getenv("SKILLS_SNAPSHOT"); out != "" {
			savePNG(t, out+"."+name+".png", c)
		}
	}
	hover(2)
	capture("physical")
	inventoryClick(t, c, f.Tabs[1])
	if len(f.List.Items) != 2 || string(f.List.Items[0]) != "Blast Attack" || string(f.List.Items[1]) != "Fire Dance" {
		t.Fatal("magical category/stunt alias", f.List.Items)
	}
	hover(1)
	capture("magical")
	inventoryClick(t, c, f.Tabs[2])
	hover(0)
	capture("assistant")
	if len(f.List.Items) != 1 || string(f.List.Items[0]) != "Slowdown" {
		t.Fatal("assistant list", f.List.Items)
	}
	inventoryClick(t, c, f.Tabs[3])
	c.Frame()
	capture("life")
	if len(f.List.Items) != 0 {
		t.Fatal("unlearned Life skills shown")
	}
	inventoryClick(t, c, f.Tabs[4])
	c.Input.X, c.Input.Y = f.Left+200, f.Top+310
	c.Frame()
	capture("intro")
	if f.List.Visible || f.Scroll.Visible || f.Up.Visible || f.Down.Visible {
		t.Fatal("Intro leaked list controls")
	}
	if len(sent()) != 0 {
		t.Fatal("browsing sent resource/action requests", sent())
	}
	if !c.SkillsKey('S', 4) || f.Visible {
		t.Fatal("Ctrl+S didn't toggle")
	}
	if !c.SkillsKey('S', 4) || !f.Visible {
		t.Fatal("Ctrl+S didn't reopen")
	}
	if !c.SkillsKey(27, 0) || f.Visible {
		t.Fatal("Escape didn't close")
	}
}
func TestSkillsServerUpdatesAndSelection(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.dispatch(skillSnapshot([]byte{94, 0, 1, 0, 0, 0, 0}))
	c.Skills.Show()
	c.dispatch([]byte{5, 11, 8, 43, 0, 0, 0x88, 0x13})
	c.dispatch([]byte{8, 1, 110, 1, 2, 0, 0, 0, 8, 43, 0, 0})
	if c.SkillState.Learned[11016].Grade != 2 || c.SkillState.Learned[11016].Proficiency != 5000 {
		t.Fatal("skill progress not routed")
	}
	c.InventoryState.Pets[2] = inventory.UsePet{ID: 14156, Name: []byte("Xaolan"), Skills: [3]game.PetSkill{{ID: 11001, Grade: 1}}}
	inventoryClick(t, c, c.Skills.Next)
	if c.Skills.Selected != 3 {
		t.Fatal("pet selector skipped occupied slot")
	}
	c.dispatch([]byte{8, 2, 4, 3, 0, 110, 1, 3, 0, 0, 0, 0xf9, 0x2a, 0, 0})
	c.dispatch([]byte{8, 2, 4, 3, 0, 111, 1, 75, 0, 0, 0, 0xf9, 0x2a, 0, 0})
	if c.InventoryState.Pets[2].Skills[0].Grade != 3 || c.InventoryState.Pets[2].Skills[0].Exp != 75 {
		t.Fatal("pet skill progress")
	}
	c.dispatch([]byte{15, 2, 0x11, 0x27, 0, 0, 3})
	if c.Skills.Selected != 0 {
		t.Fatal("released pet remained selected")
	}
	c.dispatch(skillSnapshot())
	if len(c.SkillState.Learned) != 0 || len(c.Skills.List.Items) != 2 {
		t.Fatal("fresh snapshot kept old skills")
	}
	c.dispatch(selfPacket(10002, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Other"))
	if c.Skills.Visible || len(c.SkillState.Learned) != 0 {
		t.Fatal("character change retained window/skills")
	}
}
func TestSkillsScrollAndNoOptimisticActions(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	for id, d := range c.SkillState.Catalog.Definitions {
		if d.EffectLayer == 1 && d.TableOrder != 0 {
			c.SkillState.Learned[id] = skills.Progress{Grade: 1}
		}
	}
	c.Skills.Show()
	if len(c.Skills.List.Items) < 10 {
		t.Fatal("test needs multiple pages")
	}
	c.Input.Hovered = c.Skills.List
	if !c.scrollWheel(false) || c.Skills.List.TopIndex != 1 {
		t.Fatal("wheel didn't scroll skills")
	}
	c.Skills.SetTab(2)
	c.Skills.SetTab(1)
	if c.Skills.List.TopIndex != 1 {
		t.Fatal("tab scroll state lost")
	}
	c.Skills.List.HoverRow = 0
	lines := len(c.Chat.Lines)
	c.Skills.List.DblClick()
	if len(sent()) != 0 || len(c.Chat.Lines) != lines+1 {
		t.Fatal("out-of-battle use should warn without submitting an action")
	}
	c.UI.Modal = seui.NewForm(c.Env)
	c.Skills.Hide()
	c.SkillsKey('S', 4)
	if c.Skills.Visible {
		t.Fatal("modal allowed Skills to open")
	}
	c.UI.Modal = nil
	c.Skills.Show()
	c.disconnected()
	if c.Skills.Visible {
		t.Fatal("disconnect retained skills")
	}
}
