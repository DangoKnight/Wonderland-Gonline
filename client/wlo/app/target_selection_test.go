package app

import (
	"testing"

	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

func TestFieldSelectionSocialInvalidClickCancelsWithoutWalking(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	for _, mode := range []string{"pk", "team", "friend", "trade"} {
		for _, npc := range []bool{false, true} {
			c.World.NPCs = nil
			if npc {
				cx, cy := c.World.Camera()
				c.World.NPCs = map[uint16]*world.NPC{1: {ClickID: 1, Shown: true, X: cx + 100, Y: cy + 100}}
			}
			switch mode {
			case "pk":
				c.setPKTarget(true)
			case "team":
				c.setJoinTeamTarget(true)
			case "friend":
				c.setFriendTarget(true)
			case "trade":
				c.setTradeTarget(true)
			}
			c.GroundClick(100, 100)
			if c.pkTarget || c.joinTeamTarget || c.friendTarget || c.tradeTarget || c.World.JoinTeamSelection || c.World.Walking() || c.pendingNPC != nil || len(sent()) != 0 {
				t.Fatal("invalid click leaked from selection", mode, npc, sent())
			}
		}
	}
}

func TestFieldSelectionRightClickCancelsOverUIAndOutsideField(t *testing.T) {
	c, _, sent := enterBattle(t)
	g := &Game{C: c}
	for _, mode := range []string{"pk", "team", "friend", "trade", "attack", "capture", "skill", "item", "chooser"} {
		switch mode {
		case "pk":
			c.setPKTarget(true)
		case "team":
			c.setJoinTeamTarget(true)
		case "friend":
			c.setFriendTarget(true)
		case "trade":
			c.setTradeTarget(true)
		case "attack":
			c.chooseBattleCommand(battleCommandAttack)
		case "capture":
			c.chooseBattleCommand(battleCommandCapture)
		case "skill":
			b := hud.Binding{Kind: protocol.HotbarSkill, ID: skills.BasicAttack}
			c.battle.binding = &b
		case "item":
			c.battle.item = &battleItemSelection{}
		case "chooser":
			c.hotbar.target = seui.NewForm(c.Env)
			c.UI.Add(c.hotbar.target)
			c.UI.Modal = c.hotbar.target
		}
		c.Input.Hovered = c.MainButtons.Buttons[mainTeamButton]
		g.rightPress(0, -10, -10)
		if c.pkTarget || c.joinTeamTarget || c.friendTarget || c.tradeTarget || c.battle.binding != nil || c.battle.item != nil || c.hotbar.target != nil || c.UI.Modal != nil || len(sent()) != 0 {
			t.Fatal("right click failed to cancel", mode, sent())
		}
	}
}

func TestFieldSelectionBattleInvalidClickCancelsWithoutSubmission(t *testing.T) {
	c, sent := clientItemFixture(t)
	for _, mode := range []string{"attack", "capture", "skill", "item"} {
		for _, empty := range []bool{true, false} {
			switch mode {
			case "attack":
				c.chooseBattleCommand(battleCommandAttack)
			case "capture":
				c.chooseBattleCommand(battleCommandCapture)
			case "skill":
				b := c.battleBinding(skills.BasicAttack, c.remote.battle.fighters[0])
				c.battle.binding = &b
			case "item":
				c.selectBattleItem(1, false)
			}
			x, y := 0, 0
			if !empty {
				cell := clientbattle.Cell{X: 4, Y: 2} // Allies are invalid for attack/capture.
				if mode == "item" {
					cell = clientbattle.Cell{X: 2, Y: 2}
				} // Recovery cannot target enemies.
				at := cell.Position()
				x, y = at.X, at.Y-30
			}
			c.GroundClick(x, y)
			if c.battle.binding != nil || c.battle.item != nil || len(sent()) != 0 || len(c.hotbar.submitted) != 0 || c.InventoryState.Bag[0].Count != 2 {
				t.Fatal("invalid battle target did not cancel safely", mode, empty, sent())
			}
		}
	}
}
