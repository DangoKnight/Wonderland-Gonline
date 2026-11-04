package app

import (
	"bytes"
	"os"
	"testing"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
)

func TestBattleRadialSkillsNativeWindowAndSingleClick(t *testing.T) {
	c, _, sent := enterBattle(t)
	c.SkillState.Learned[11016] = skills.Progress{Grade: 1}
	c.chooseBattleCommand(battleCommandSkills)
	f := c.BattleSkills
	if !f.Visible || c.Skills.Visible || f.Image < 0 || f.Image != c.Pics.Find("Form_BattleSkill_1") || f.Width != 274 || f.Height != 353 {
		t.Fatal("radial skills did not open the native battle form")
	}
	if len(f.List.Items) != 1 || string(f.List.Items[0]) != "Flame Attack" || len(f.Recent.Items) != 0 || len(sent()) != 0 {
		t.Fatal("battle skill list or unsolicited request", f.List.Items, sent())
	}
	for _, b := range []*seui.FixedButton{f.Up, f.Down, f.Close} {
		if b.Image < 0 {
			t.Fatal("missing battle skill control artwork")
		}
	}
	at := f.List.Abs()
	c.Input.X, c.Input.Y = at.X+70, at.Y+10
	c.Frame()
	c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if f.Visible || c.battle.binding == nil || c.battle.binding.ID != 11016 || len(sent()) != 0 {
		t.Fatal("one click did not arm battle skill targeting")
	}
	c.HotbarKey(escapeKey, 0)
	c.chooseBattleCommand(battleCommandSkills)
	if len(f.Recent.Items) != 1 || string(f.Recent.Items[0]) != "Flame Attack" {
		t.Fatal("missing recent skill", f.Recent.Items)
	}
	f.Recent.OnSelect(0)
	c.GroundClick(230, 370)
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{50, 1, 4, 2, 2, 2, 8, 43}) {
		t.Fatal("battle skill request", p)
	}
}

func TestBattleSkillsCloseAndGeneralModuleIndependent(t *testing.T) {
	c, _, _ := enterBattle(t)
	c.chooseBattleCommand(battleCommandSkills)
	if !c.SkillsKey(escapeKey, 0) || c.BattleSkills.Visible {
		t.Fatal("escape did not close battle skills")
	}
	c.toggleSkills()
	if !c.Skills.Visible || c.BattleSkills.Visible {
		t.Fatal("general Skills module replaced by battle form")
	}
	c.chooseBattleCommand(battleCommandSkills)
	c.closeBattleView()
	if c.BattleSkills.Visible {
		t.Fatal("battle exit retained skill window")
	}
}

func TestBattleSkillsReferenceSnapshot(t *testing.T) {
	out := os.Getenv("BATTLE_SKILLS_SNAPSHOT")
	if out == "" {
		t.Skip("set BATTLE_SKILLS_SNAPSHOT")
	}
	c, _, _ := enterBattle(t)
	for id, d := range c.SkillState.Catalog.Definitions {
		switch d.Name {
		case "Flame Attack", "Blast Attack", "Slowdown", "Love Wish":
			c.SkillState.Learned[id] = skills.Progress{Grade: 1}
		}
	}
	c.chooseBattleCommand(battleCommandSkills)
	f := c.BattleSkills
	if len(f.List.Items) > 0 {
		f.List.OnSelect(0)
		c.HotbarKey(escapeKey, 0)
		c.chooseBattleCommand(battleCommandSkills)
	}
	c.Frame()
	savePNG(t, out, c)
}
