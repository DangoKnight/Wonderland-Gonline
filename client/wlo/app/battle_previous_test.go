package app

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
)

func previousTestNextTurn(c *Client) {
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.battleFrame(false)
}

func TestBattlePreviousOffenseChoosesAutoTargetInsteadOfOriginalTarget(t *testing.T) {
	fixture := newClientFixture(t)
	for _, tc := range []struct {
		name  string
		id    uint16
		layer byte
	}{
		{"attack", skills.BasicAttack, skills.Physical},
		{"physical skill", autoTestAttackSkill, skills.Physical},
		{"magic skill", autoTestAttackSkill, skills.Magical},
		{"debuff skill", autoTestAttackSkill, nativeDebuffLayer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, sent := enterBattleWith(t, fixture)
			autoTestDefinition(c, tc.id, tc.layer, 0)
			c.SkillState.Learned[tc.id] = skills.Progress{Grade: 1}
			// The manually chosen target B is (2,2). AUTO's target A at (1,1)
			// is deliberately appended later to the packet/participant list.
			enemy := remoteRecord(5, 7, 1, 1, 20002, 0)
			enemy[2] = 2
			c.dispatch(enemy)
			c.useBinding(c.battleBinding(tc.id, c.remote.battle.fighters[0]))
			c.GroundClick(230, 370)
			if p := sent(); len(p) != 1 || p[0][4] != 2 || p[0][5] != 2 {
				t.Fatal("manual action did not choose target B", p)
			}
			c.dispatch([]byte{53, 5, 4, 2})
			previousTestNextTurn(c)
			c.battle.previous.OnClick()
			want := []byte{50, 1, 4, 2, 1, 1, byte(tc.id), byte(tc.id >> 8)}
			if p := sent(); len(p) != 2 || !bytes.Equal(p[1], want) || c.battle.binding != nil || c.battle.auto {
				t.Fatal("previous action failed to submit one fresh AUTO target", p)
			}
			c.chooseBattleCommand(battleCommandPrevious)
			if len(sent()) != 2 {
				t.Fatal("previous action submitted twice")
			}
		})
	}
}

func TestBattlePreviousPlayerAndPetHaveIndependentRadialActions(t *testing.T) {
	for _, chain := range []bool{false, true} {
		t.Run(fmt.Sprint("chain=", chain), func(t *testing.T) {
			c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
			c.InventoryState.Pets[0].ID = 14162
			autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, skills.BasicAttack)
			autoTestAcknowledge(c, clientbattle.Cell{X: 3, Y: 2}, skills.Defense)
			previousTestNextTurn(c)
			if chain {
				c.combat.enabled = true
				c.dispatch([]byte{254, 3, 9, 0, 0, 0, 4, 2})
			}
			player := c.battle.previousActions[clientbattle.Cell{X: 4, Y: 2}]
			pet := c.battle.previousActions[clientbattle.Cell{X: 3, Y: 2}]
			if !player.form.Visible || pet.form.Visible || player.button.Image != c.Pics.Find("Btn_Battle_6_1") || pet.button.Image != c.Pics.Find("Btn_Battle_1_1") {
				t.Fatal("missing independent previous action icons")
			}
			for cell, control := range map[clientbattle.Cell]battlePreviousControl{{X: 4, Y: 2}: player, {X: 3, Y: 2}: pet} {
				at := cell.Position()
				if p := control.button.Abs(); p.X != at.X-battleCommandOriginX+65 || p.Y != at.Y-battleCommandOriginY+83 || control.form.Draggable {
					t.Fatal("previous action is not in the radial center", p)
				}
			}
			if !chain {
				if path := os.Getenv("BATTLE_PREVIOUS_SNAPSHOT"); path != "" {
					c.Frame()
					savePNG(t, path, c)
				}
			}
			// Selecting the pet replaces the center shortcut with its own history.
			c.battle.selected = clientbattle.Cell{X: 3, Y: 2}
			c.battleMenu()
			if player.form.Visible || !pet.form.Visible {
				t.Fatal("radial shortcut did not follow selection")
			}
			pet.button.OnClick()
			if pet.form.Visible || c.battle.auto || c.battle.binding != nil {
				t.Fatal("pet shortcut retained the submitted control or enabled AUTO")
			}
			player.button.OnClick()
			pet.button.OnClick()
			player.button.OnClick()
			want := [][]byte{{50, 4, 3, 2, 3, 2, 0x75, 0xea}, {50, 1, 4, 2, 2, 2, 0x11, 0x27}}
			if chain {
				want = [][]byte{{254, 2, 9, 0, 0, 0, 2, 50, 4, 3, 2, 3, 2, 0x75, 0xea, 50, 1, 4, 2, 2, 2, 0x11, 0x27}}
			}
			got := sent()
			if len(got) != len(want) {
				t.Fatal("missing or duplicate shortcut choices", got)
			}
			for i := range want {
				if !bytes.Equal(got[i], want[i]) {
					t.Fatalf("choice %d: %v want %v", i, got[i], want[i])
				}
			}
			c.closeBattleView()
			if player.form.Visible || pet.form.Visible || c.battle.previousActions != nil {
				t.Fatal("battle exit retained previous action controls")
			}
		})
	}
}

func TestBattlePreviousSpecialActions(t *testing.T) {
	fixture := newClientFixture(t)
	for _, tc := range []struct {
		name  string
		skill uint16
		icon  string
		want  []byte
	}{
		{"defend", skills.Defense, "Btn_Battle_1_1", []byte{50, 4, 4, 2, 4, 2, 0x75, 0xea}},
		{"flee", battleFleeSkill, "Btn_Battle_3_1", []byte{50, 5, 4, 2, 4, 2, 0x89, 0xea}},
		{"capture", battleCatchSkill, "Btn_Battle_2_1", []byte{50, 1, 4, 2, 2, 2, 0x18, 0x27}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, sent := enterBattleWith(t, fixture)
			autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, tc.skill)
			previousTestNextTurn(c)
			if c.battle.previous.Image != c.Pics.Find(tc.icon) {
				t.Fatal("incorrect previous-action icon")
			}
			c.battle.previous.OnClick()
			if tc.skill == battleCatchSkill {
				if len(sent()) != 0 || c.battle.binding == nil || c.battle.binding.ID != battleCatchSkill {
					t.Fatal("capture did not retain manual target selection")
				}
				c.GroundClick(230, 370)
			}
			if got := sent(); len(got) != 1 || !bytes.Equal(got[0], tc.want) {
				t.Fatal("incorrect previous special action", got)
			}
		})
	}
}

func TestBattlePreviousSkillValidationAndTargetAvailability(t *testing.T) {
	fixture := newClientFixture(t)
	for _, reason := range []string{"low SP", "forgotten", "unknown", "no enemies", "support", "playback"} {
		t.Run(reason, func(t *testing.T) {
			c, _, sent := enterBattleWith(t, fixture)
			layer := byte(skills.Magical)
			if reason == "support" {
				layer = autoTestSupportLayer
			}
			autoTestDefinition(c, autoTestAttackSkill, layer, 6)
			c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
			autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, autoTestAttackSkill)
			previousTestNextTurn(c)
			button := c.battle.previous
			switch reason {
			case "low SP":
				c.dispatch([]byte{51, 1, 4, 2, 26, 0, 0, 0, 0})
			case "forgotten":
				delete(c.SkillState.Learned, autoTestAttackSkill)
			case "unknown":
				delete(c.SkillState.Catalog.Definitions, autoTestAttackSkill)
			case "no enemies":
				c.battle.state.At(clientbattle.Cell{X: 2, Y: 2}).Removed = true
			case "playback":
				c.battle.playing = []battlePlay{{}}
			}
			button.OnClick()
			if len(sent()) != 0 {
				t.Fatal("unavailable previous skill submitted", sent())
			}
			if reason == "support" {
				if c.battle.binding == nil || c.battle.binding.ID != autoTestAttackSkill {
					t.Fatal("support shortcut lost normal ally targeting")
				}
			} else if c.battle.binding != nil {
				t.Fatal("offensive shortcut requested manual target selection")
			}
		})
	}
}

func TestBattlePreviousCaptureFailureRetainsCaptureCommand(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 0x19, 0x27, 0, 1, 2, 2, 1, 0, 1, 25, 0, 0, 0, 0, 1})
	if c.battle.lastActions[clientbattle.Cell{X: 4, Y: 2}].skill != battleCatchSkill {
		t.Fatal("capture failure animation replaced the repeatable capture command")
	}
	c.battleFrame(false)
	*now = now.Add(time.Minute)
	c.battleFrame(false)
	previousTestNextTurn(c)
	if c.battle.previous == nil || c.battle.previous.Image != c.Pics.Find("Btn_Battle_2_1") {
		t.Fatal("capture failure retained an invalid previous-action icon")
	}
}

func TestBattlePreviousButtonConsumesPointerClick(t *testing.T) {
	c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.InventoryState.Pets[0].ID = 14162
	autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, skills.BasicAttack)
	autoTestAcknowledge(c, clientbattle.Cell{X: 3, Y: 2}, skills.Defense)
	previousTestNextTurn(c)
	for i, cell := range []clientbattle.Cell{{X: 3, Y: 2}, {X: 4, Y: 2}} {
		c.battle.selected = cell
		c.battleMenu()
		button := c.battle.previousActions[cell].button
		at := button.Abs()
		c.Input.X, c.Input.Y = at.X+14, at.Y+14
		c.Frame()
		if c.Input.Hovered != button {
			t.Fatal("previous action shortcut is not reachable in the radial center", cell, c.Input.Hovered)
		}
		c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
		c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
		if len(sent()) != i+1 || c.battle.binding != nil {
			t.Fatal("previous shortcut click did not submit exactly one action", sent())
		}
	}
}

func TestBattlePreviousSkipsUnavailableAutoTargets(t *testing.T) {
	c, _, sent := enterBattle(t)
	enemy := remoteRecord(5, 7, 1, 1, 20002, 0)
	enemy[2] = 2
	c.dispatch(enemy)
	autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, skills.BasicAttack)
	for i, reason := range []string{"dead", "removed", "waiting"} {
		previousTestNextTurn(c)
		first := c.battle.state.At(clientbattle.Cell{X: 1, Y: 1})
		first.HP, first.Removed, first.Waiting = 100, false, false
		for j := range c.remote.battle.fighters {
			if c.remote.battle.fighters[j].x == 1 && c.remote.battle.fighters[j].y == 1 {
				c.remote.battle.fighters[j].hp = 100
				if reason == "dead" {
					c.remote.battle.fighters[j].hp = 0
				}
			}
		}
		switch reason {
		case "dead":
			first.HP = 0
		case "removed":
			first.Removed = true
		case "waiting":
			first.Waiting = true
		}
		c.battle.previous.OnClick()
		if p := sent(); len(p) != i+1 || !bytes.Equal(p[i], []byte{50, 1, 4, 2, 2, 2, 17, 39}) {
			t.Fatal("previous action did not skip", reason, p)
		}
	}
}

func TestBattlePreviousStalePetButtonCannotControlReplacement(t *testing.T) {
	c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.InventoryState.Pets[0].ID = 14162
	cell := clientbattle.Cell{X: 3, Y: 2}
	autoTestAcknowledge(c, cell, skills.BasicAttack)
	previousTestNextTurn(c)
	old := c.battle.previousActions[cell]
	// Simulate the synchronized roster after another pet reuses this cell.
	c.InventoryState.Pets[0].ID = 14163
	c.battle.state.At(cell).ID = 14163
	for i := range c.remote.battle.fighters {
		if c.remote.battle.fighters[i].x == cell.X && c.remote.battle.fighters[i].y == cell.Y {
			c.remote.battle.fighters[i].id = 14163
		}
	}
	c.saveBattleAction(cell, battlePreviousAction{14163, 10001, clientbattle.Pet, skills.Defense})
	c.battleMenu()
	current := c.battle.previousActions[cell]
	old.button.OnClick()
	if old.form.Visible || current.button == old.button || len(sent()) != 0 {
		t.Fatal("stale pet shortcut affected the replacement", sent())
	}
	current.button.OnClick()
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{50, 4, 3, 2, 3, 2, 0x75, 0xea}) {
		t.Fatal("replacement pet lost its own previous action", p)
	}
}
