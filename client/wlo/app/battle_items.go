package app

import (
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

type battleItemSelection struct {
	actor      clientbattle.Cell
	generation uint64
	slot       byte
	item       game.Item
}

// Inventory double-click selects an item without spending it. Like native
// FUN_0039be38, the subsequent click on a fighter submits AC50:2.
func (c *Client) selectBattleItem(slot int, worn bool) bool {
	if !c.battle.state.Active {
		return false
	}
	if worn || slot < 1 || slot > game.BagSize || c.battleAnimating() || c.remote.active && c.remote.options.AutoFight {
		return true
	}
	actor, ok := c.battleActor()
	if !ok {
		return true
	}
	it := c.InventoryState.Bag[slot-1]
	if it.Empty() || it.Locked {
		return true
	}
	if _, ok := c.items[it.ID].BattleItem(); !ok {
		c.Chat.Notice("This item cannot be used in battle.")
		return true
	}
	cell := clientbattle.Cell{X: actor.x, Y: actor.y}
	c.battle.item = &battleItemSelection{actor: cell, generation: c.battle.state.At(cell).Generation, slot: byte(slot), item: it}
	c.battle.binding = nil
	if c.battle.menu != nil {
		c.battle.menu.Visible = false
	}
	c.Inventory.Hide()
	return true
}

func (c *Client) submitBattleItem(target remoteFighter) {
	selected := c.battle.item
	if selected == nil || c.battleAnimating() {
		return
	}
	actor := c.battle.state.At(selected.actor)
	if actor == nil || actor.Generation != selected.generation || !c.battle.state.CanChoose(selected.actor) || c.hotbar.submitted[[2]byte{actor.X, actor.Y}] || c.InventoryState.Bag[selected.slot-1] != selected.item {
		c.battle.item = nil
		return
	}
	info, ok := c.items[selected.item.ID].BattleItem()
	if !ok {
		return
	}
	t := c.battle.state.At(clientbattle.Cell{X: target.x, Y: target.y})
	if t == nil || t.Removed || t.Waiting {
		return
	}
	if info.Skill != 0 {
		d, known := c.SkillState.Catalog.Definitions[info.Skill]
		if !known || d.TargetKnown && !d.AllowsTarget(t.Side == actor.Side, t.Cell == actor.Cell) {
			return
		}
	}
	// Ordinary supplies target living allies; linked battle-item skills use
	// their native target classification from the existing skill catalog.
	if info.Skill == assets.BattleItemReviveSkill {
		if t.Side != actor.Side || t.HP != 0 {
			return
		}
	} else {
		if t.HP == 0 {
			return
		}
		if info.Skill == 0 && t.Side != actor.Side {
			return
		}
	}
	p := protocol.BattleItemChoice(actor.X, actor.Y, t.X, t.Y, selected.item.ID, selected.slot)
	if err := c.sendCombatChoice(p); err != nil {
		c.Chat.Notice(err.Error())
		return
	}
	if c.hotbar.submitted == nil {
		c.hotbar.submitted = map[[2]byte]bool{}
	}
	c.hotbar.submitted[[2]byte{actor.X, actor.Y}] = true
	if c.battle.itemActors == nil {
		c.battle.itemActors = map[clientbattle.Cell]bool{}
	}
	c.battle.itemActors[actor.Cell] = true
	c.forgetBattleAction(actor.Cell)
	c.battle.item = nil
}
