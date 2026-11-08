package app

import (
	"encoding/binary"
	"sort"
	"strconv"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/protocol"
)

func (c *Client) remoteChoices(pet bool) []inventory.RemoteChoice {
	if c.SkillState == nil || c.SkillState.Catalog == nil {
		return nil
	}
	target := byte(0)
	if pet {
		target = c.remotePetSlot()
		if target == 0 {
			return nil
		}
	}
	var choices []inventory.RemoteChoice
	for id, d := range c.SkillState.Catalog.Definitions {
		if !c.bindingLearned(c.skillBinding(id, target)) && id != battleFleeSkill || d.EffectLayer == skills.LifeLayer {
			continue
		}
		choices = append(choices, inventory.RemoteChoice{ID: id, Name: clientassets.Big5Text(d.Name), Icon: d.IconName()})
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	seen := map[uint16]bool{}
	for _, item := range c.InventoryState.Bag {
		if item.Empty() || item.Locked || seen[item.ID] {
			continue
		}
		d := c.items[item.ID]
		if _, ok := d.BattleItem(); !ok {
			continue
		}
		seen[item.ID] = true
		choices = append(choices, inventory.RemoteChoice{ID: item.ID, Item: true, Name: clientassets.Big5Text(d.Definition.Name), Icon: strconv.Itoa(int(d.Icon))})
	}
	for _, choice := range choices {
		c.loadPictures(choice.Icon)
	}
	return choices
}

// FUN_003b66dc evaluates emergency/revive, enemy HP, HP recovery, SP recovery,
// three one-shot sequence slots, then two default attacks. Player and pet have
// independent configurations and sequence cursors. All choices use native AC50.
func (c *Client) remoteAdvancedAction(actor remoteFighter) bool {
	index := 0
	if actor.kind == remoteFighterPet {
		index = 1
	}
	a := c.remote.options.Advanced[index]
	if !a.Active() || c.SkillState == nil {
		return false
	}
	try := func(slot int, target byte, accepts func(remoteFighter) bool) bool {
		assignment := a.Assignments[slot]
		if assignment.ID == 0 {
			return false
		}
		if assignment.Item && (slot == 5 || slot == 6) {
			info, ok := c.items[assignment.ID].BattleItem()
			if !ok || info.Skill == 0 && (slot == 5 && info.HP <= 0 || slot == 6 && info.SP <= 0) {
				return false
			}
		}
		if target != 0 {
			assignment.Target = target
		}
		t, ok := c.remoteAdvancedTarget(actor, assignment, accepts)
		return ok && c.remoteSubmitAssignment(actor, t, assignment)
	}
	if a.Enabled[inventory.RemoteEmergency] && try(7, inventory.RemoteTargetParty, func(t remoteFighter) bool { return t.hp == 0 }) {
		return true
	}
	if a.Enabled[inventory.RemoteEnemyHP] {
		for _, enemy := range c.remote.battle.fighters {
			if enemy.side == actor.side || enemy.hp == 0 {
				continue
			}
			if (!a.EnemyAbove && uint64(enemy.hp) < uint64(a.EnemyHP)) || (a.EnemyAbove && uint64(enemy.hp) > uint64(a.EnemyHP)) {
				if try(8, 0, nil) {
					return true
				}
			}
			break // Native condition tests the next living enemy, not any arbitrary match.
		}
	}
	if a.Enabled[inventory.RemoteHealHP] && try(5, 0, func(t remoteFighter) bool { return t.hp > 0 && t.hp < t.maxHP && uint64(t.hp) < uint64(a.HP) }) {
		return true
	}
	if a.Enabled[inventory.RemoteHealSP] && try(6, 0, func(t remoteFighter) bool { return t.hp > 0 && t.sp < t.maxSP && int(t.sp) < a.SP }) {
		return true
	}
	if a.Enabled[inventory.RemoteSequence] {
		for c.remote.sequence[index] < 3 {
			slot := c.remote.sequence[index] + 2
			c.remote.sequence[index]++
			if try(slot, 0, nil) {
				return true
			}
		}
	}
	if a.Enabled[inventory.RemoteDefault] {
		for slot := 0; slot < 2; slot++ {
			if !a.Assignments[slot].Item && try(slot, inventory.RemoteTargetEnemy, nil) {
				return true
			}
		}
	}
	return false
}

func (c *Client) remoteAdvancedTarget(actor remoteFighter, a inventory.RemoteAssignment, accepts func(remoteFighter) bool) (remoteFighter, bool) {
	selfOnly := false
	if !a.Item {
		d := c.SkillState.Catalog.Definitions[a.ID]
		selfOnly = a.ID == skills.Defense || a.ID == battleFleeSkill || d.TargetKnown && d.NativeTarget == skills.TargetSelf
		if selfOnly && accepts == nil {
			return actor, true
		}
	}
	policy := a.Target
	if policy == 0 {
		policy = inventory.RemoteTargetEnemy
		if accepts != nil {
			policy = inventory.RemoteTargetParty
		}
	}
	for _, target := range c.remote.battle.fighters {
		f := c.battle.state.At(clientbattle.Cell{X: target.x, Y: target.y})
		if f == nil || f.Waiting || f.Removed {
			continue
		}
		switch policy {
		case inventory.RemoteTargetEnemy:
			if target.side == actor.side || target.hp == 0 {
				continue
			}
		case inventory.RemoteTargetPlayer:
			if target.kind != remoteFighterPlayer || target.id != c.World.Player.ID {
				continue
			}
		case inventory.RemoteTargetPet:
			if target.kind != remoteFighterPet || target.owner != c.World.Player.ID {
				continue
			}
		case inventory.RemoteTargetParty:
			if target.side != actor.side {
				continue
			}
		default:
			continue
		}
		if accepts != nil && !accepts(target) {
			continue
		}
		if selfOnly {
			return actor, true
		}
		if !a.Item {
			binding := c.battleBinding(a.ID, actor)
			// Native conditions inspect the selected ally first, then redirect
			// an offensive skill to an enemy rather than attacking that ally.
			if accepts != nil && c.battleBindingTargetsEnemy(binding) {
				return c.battleAutoTarget(binding, actor)
			}
			if !c.hotbarTargetAllowed(binding, actor, target) {
				continue
			}
		}
		return target, true
	}
	return remoteFighter{}, false
}

func (c *Client) remoteSubmitAssignment(actor, target remoteFighter, a inventory.RemoteAssignment) bool {
	if a.Item {
		for i, item := range c.InventoryState.Bag {
			if item.ID != a.ID || item.Empty() || item.Locked || uint16(item.Count) <= c.remote.reservedItems[byte(i+1)] {
				continue
			}
			info, ok := c.items[item.ID].BattleItem()
			if !ok {
				return false
			}
			if info.Skill == assets.BattleItemReviveSkill {
				if target.side != actor.side || target.hp != 0 {
					return false
				}
			} else if target.hp == 0 || info.Skill == 0 && target.side != actor.side {
				return false
			}
			if info.Skill != 0 {
				d, known := c.SkillState.Catalog.Definitions[info.Skill]
				if !known || d.TargetKnown && !d.AllowsTarget(target.side == actor.side, target.x == actor.x && target.y == actor.y) {
					return false
				}
			}
			p := protocol.BattleItemChoice(actor.x, actor.y, target.x, target.y, item.ID, byte(i+1))
			if err := c.sendCombatChoice(p); err != nil {
				c.stopRemote()
				c.Chat.Notice(err.Error())
			} else {
				if c.remote.reservedItems == nil {
					c.remote.reservedItems = map[byte]uint16{}
				}
				c.remote.reservedItems[byte(i+1)]++
			}
			return true
		}
		return false
	}
	binding := c.battleBinding(a.ID, actor)
	if actor.kind == remoteFighterPet && binding.Target == 0 {
		return false
	}
	d, known := c.SkillState.Catalog.Definitions[c.bindingSkillID(binding)]
	if !known || !c.bindingLearned(binding) && a.ID != battleFleeSkill || d.SP > actor.sp {
		return false
	}
	id := c.bindingSkillID(binding)
	sub := byte(remoteBattleAttack)
	if id == skills.Defense {
		sub = hotbarDefendAction
	}
	if id == battleFleeSkill {
		sub = protocol.BattleActionFleeRequest
	}
	p := []byte{protocol.CommandBattleAction, sub, actor.x, actor.y, target.x, target.y}
	p = binary.LittleEndian.AppendUint16(p, id)
	if err := c.sendCombatChoice(p); err != nil {
		c.stopRemote()
		c.Chat.Notice(err.Error())
	}
	return true
}
