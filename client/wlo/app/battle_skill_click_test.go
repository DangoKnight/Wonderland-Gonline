package app

import (
	"bytes"
	"testing"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
)

func skillListPress(t *testing.T, c *Client) {
	t.Helper()
	c.Skills.Show()
	at := c.Skills.List.Abs()
	c.Input.X, c.Input.Y = at.X+75, at.Y+10
	c.Input.Hovered = nil
	c.Frame()
	if c.Input.Hovered != c.Skills.List {
		t.Fatal("skill list not reachable")
	}
	c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
}

func TestBattleSkillSingleClickArmsTargeting(t *testing.T) {
	c, _, sent := enterBattle(t)
	skillListPress(t, c)
	if c.dropHotbar(c.Input.X, c.Input.Y) {
		t.Fatal("stationary click counted as drag")
	}
	c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if c.Skills.Visible || c.battle.binding == nil || c.battle.binding.ID != skills.BasicAttack || len(sent()) != 0 {
		t.Fatal("one click did not arm skill targeting")
	}
	c.GroundClick(230, 370)
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{50, 1, 4, 2, 2, 2, 17, 39}) {
		t.Fatal("skill target request", p)
	}
}

func TestBattleSkillDragStillAssignsHotbar(t *testing.T) {
	c, _, sent := enterBattle(t)
	skillListPress(t, c)
	at := c.HotKeys.Slots[1].Abs()
	c.Input.X, c.Input.Y = at.X+10, at.Y+10
	c.Frame()
	if !c.dropHotbar(c.Input.X, c.Input.Y) {
		t.Fatal("drag not consumed")
	}
	if c.battle.binding != nil || !c.Skills.Visible || c.HotKeys.Bindings[1][1].ID != skills.BasicAttack {
		t.Fatal("drag activated skill")
	}
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{40, 1, 2, 17, 39, 1, 1}) {
		t.Fatal("drag assignment", p)
	}
}

func TestSkillsOutsideBattleClickOnlyBrowses(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	calls := 0
	c.Skills.Use = func(uint16, byte) { calls++ }
	skillListPress(t, c)
	c.dropHotbar(c.Input.X, c.Input.Y)
	c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if calls != 0 || !c.Skills.Visible {
		t.Fatal("browsing used skill")
	}
	c.Skills.List.DblClick()
	if calls != 1 {
		t.Fatal("ordinary double click stopped working", calls)
	}
}
