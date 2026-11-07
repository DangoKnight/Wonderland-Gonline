package app

import (
	"fmt"

	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const (
	battlePreviousX = 65
	battlePreviousY = 83
)

type battlePreviousControl struct {
	form     *seui.Form
	button   *seui.FixedButton
	identity battleActionIdentity
}

// Show the selected unit's acknowledged previous action in the radial center.
// Player and pet histories remain independent when selection moves between them.
func (c *Client) battlePreviousActions(show bool, selected remoteFighter) {
	v := &c.battle
	v.previous = nil
	for cell, control := range v.previousActions {
		control.form.Visible, control.button.Visible = false, false
		if cell == (clientbattle.Cell{X: selected.x, Y: selected.y}) && control.identity == (battleActionIdentity{selected.id, selected.owner, selected.kind}) {
			v.previous = control.button
		}
	}
	if !show {
		return
	}
	if v.previousActions == nil {
		v.previousActions = map[clientbattle.Cell]battlePreviousControl{}
	}
	for _, actor := range c.remote.battle.fighters {
		cell := clientbattle.Cell{X: actor.x, Y: actor.y}
		if f := v.state.At(cell); !v.state.Owned(f) {
			continue
		}
		identity := battleActionIdentity{actor.id, actor.owner, actor.kind}
		control, exists := v.previousActions[cell]
		if !exists || control.identity != identity {
			if exists {
				c.UI.Remove(control.form)
			}
			form := seui.NewForm(c.Env)
			form.Dockable, form.Draggable = false, false
			form.Init("", 0, 0, 0, 0, 0, false, battleCommandButtonSize, battleCommandButtonSize, 0)
			form.PixelHit = true
			button := seui.NewFixedButton(c.Env, form)
			button.Init("", 0, battleCommandButtonSize, battleCommandButtonSize, 0, 0, true, battleCommandButtonSize, battleCommandButtonSize, 0)
			form.Visible, button.Visible = false, false
			button.BindLanguage("hint", func() {
				button.SetHint([]byte(localization.UI("ui.app.repeat_last_action_8d9336f7")))
			})
			button.OnClick = func() {
				// Revalidate the identity: a delayed click must not control a
				// replacement pet that has since reused the same formation cell.
				for _, current := range c.remote.battle.fighters {
					if current.x == cell.X && current.y == cell.Y &&
						(battleActionIdentity{current.id, current.owner, current.kind}) == identity {
						c.repeatBattleAction(current)
						return
					}
				}
			}
			control = battlePreviousControl{form, button, identity}
			v.previousActions[cell] = control
			c.UI.Add(form)
		}
		if actor.x == selected.x && actor.y == selected.y {
			v.previous = control.button
		}
		last, known := v.lastActions[cell]
		if !known || last.skill == 0 || last.id != actor.id || last.owner != actor.owner || last.kind != actor.kind ||
			!v.state.CanChoose(cell) || c.hotbar.submitted[[2]byte{actor.x, actor.y}] {
			continue
		}
		icon := battleIconSkills
		switch last.skill {
		case skills.BasicAttack:
			icon = battleIconAttack
		case skills.Defense:
			icon = battleIconDefend
		case battleCatchSkill:
			icon = battleIconCapture
		case battleFleeSkill:
			icon = battleIconFlee
		}
		control.button.Image = c.Pics.Find(fmt.Sprintf("Btn_Battle_%d_1", icon))
		at := cell.Position()
		control.form.Left = at.X - battleCommandOriginX + battlePreviousX
		control.form.Top = at.Y - battleCommandOriginY + battlePreviousY
		visible := actor.x == selected.x && actor.y == selected.y
		control.form.Visible, control.button.Visible = visible, visible
	}
}

func (c *Client) repeatBattleAction(actor remoteFighter) {
	v := &c.battle
	cell := clientbattle.Cell{X: actor.x, Y: actor.y}
	if !v.state.CanChoose(cell) || c.hotbar.submitted[[2]byte{actor.x, actor.y}] || c.battleAnimating() ||
		v.timedOut || v.auto || v.binding != nil || v.item != nil || c.remote.active && c.remote.options.AutoFight {
		return
	}
	last, ok := v.lastActions[cell]
	if !ok || last.skill == 0 || last.id != actor.id || last.owner != actor.owner || last.kind != actor.kind {
		return
	}
	v.selected = cell
	binding := c.battleBinding(last.skill, actor)
	switch last.skill {
	case battleCatchSkill:
		v.binding = &binding
	case battleFleeSkill:
		c.submitBattleSpecial(actor, actor, last.skill, protocol.BattleActionFleeRequest)
	default:
		if _, known := c.SkillState.Catalog.Definitions[c.bindingSkillID(binding)]; !known && binding.ID != skills.BasicAttack && binding.ID != skills.Defense {
			c.Chat.Notice(localization.UI("ui.app.this_skill_is_no_longer_available_to_the_4133b998"))
			return
		}
		// Reuse the normal learned-skill, SP and ownership validation. Ally
		// skills retain manual targeting; offensive skills choose AUTO's target.
		c.useBinding(binding)
		if v.binding != nil && c.battleBindingTargetsEnemy(binding) {
			v.binding = nil
			if target, found := c.battleAutoTarget(binding, actor); found {
				c.submitHotbar(binding, target)
			} else {
				c.Chat.Notice(localization.UI("ui.app.no_available_target_d7cdd7f3"))
			}
		}
	}
	c.battleMenu()
}

// Shared by AUTO and one-click previous actions. Scan native enemy columns and
// rows, filtering dead, removed, waiting and otherwise invalid targets.
func (c *Client) battleAutoTarget(binding hud.Binding, actor remoteFighter) (remoteFighter, bool) {
	columns := [2]byte{4, 3}
	if actor.x >= 3 {
		columns = [2]byte{1, 2}
	}
	for _, x := range columns {
		for y := byte(1); y <= clientbattle.GridRows; y++ {
			for _, target := range c.remote.battle.fighters {
				if target.x == x && target.y == y && target.hp > 0 && c.hotbarTargetAllowed(binding, actor, target) {
					return target, true
				}
			}
		}
	}
	return remoteFighter{}, false
}
