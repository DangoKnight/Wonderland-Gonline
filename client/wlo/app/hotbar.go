package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"fmt"
	"image"
	"path/filepath"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	hotbarFirstKey             = 0x70
	hotbarLastKey              = 0x77
	hotbarDragDistance         = 4
	hotbarTargetWidth          = 300
	hotbarTargetRow            = 24
	hotbarNativeRecordBytes    = 5
	hotbarDefendAction         = 4
	hotbarTargetInk            = 0x001f
	nativeControlLayer         = 3
	nativeDebuffLayer          = 15
	nativeDebuffAlternateLayer = 16
	nativeReviveLayer          = 8
)

type hotbarDrag struct {
	binding                hud.Binding
	start                  image.Point
	sourcePage, sourceSlot byte
}
type hotbarRuntime struct {
	path      string
	drag      *hotbarDrag
	target    *seui.Form
	submitted map[[2]byte]bool
	turn      uint64
}

func (c *Client) initHotbar() {
	c.HotKeys.Describe = c.describeBinding
	c.HotKeys.Use = c.useBinding
	c.HotKeys.Clear = func(slot byte) { c.assignBinding(c.HotKeys.Page, slot, hud.Binding{}) }
	c.HotKeys.BeginDrag = func(slot byte, b hud.Binding) {
		if b.ID != 0 && c.UI.Modal == nil {
			c.hotbar.drag = &hotbarDrag{binding: b, start: image.Pt(c.Input.X, c.Input.Y), sourcePage: c.HotKeys.Page, sourceSlot: slot}
		}
	}
	c.Skills.List.OnDown = func() {
		id, target := c.Skills.SkillAt(c.Input.X, c.Input.Y)
		if id != 0 {
			c.hotbar.drag = &hotbarDrag{binding: c.skillBinding(id, target), start: image.Pt(c.Input.X, c.Input.Y)}
		}
	}
	c.Skills.Use = func(id uint16, target byte) { c.useBinding(c.skillBinding(id, target)) }
}
func (c *Client) skillBinding(id uint16, target byte) hud.Binding {
	b := hud.Binding{Kind: protocol.HotbarSkill, ID: id, Target: target}
	if target != 0 && target <= game.MaxPets {
		b.Pet = c.InventoryState.Pets[target-1].ID
	}
	return b
}
func (c *Client) loadHotbar() {
	c.closeHotbarTarget()
	c.hotbar = hotbarRuntime{}
	c.HotKeys.Bindings = hud.Bindings{}
	c.HotKeys.Page = 1
	if c.World != nil {
		c.hotbar.path = filepath.Join(filepath.Dir(c.settingsPath), fmt.Sprintf("hotbar-%d.json", c.World.Player.ID))
		b, err := hud.LoadBindings(c.hotbar.path)
		if err != nil {
			c.Chat.Notice(localization.UI("ui.app.could_not_load_hotbar_1a2b8593") + err.Error())
		} else {
			c.HotKeys.Bindings = b
		}
	}
	c.HotKeys.Refresh()
}
func (c *Client) describeBinding(b hud.Binding) (string, string) {
	if b.Kind == protocol.HotbarItem {
		d := c.items[b.ID]
		icon := fmt.Sprint(d.Icon)
		c.loadPictures(icon)
		return icon, string(clientassets.Big5Text(d.Definition.Name))
	}
	id := b.ID
	if id == game.StarterStuntClientID && c.World != nil {
		id = game.StarterStunt(uint16(c.World.Player.Body), uint16(c.World.Player.Head))
	}
	d := c.SkillState.Catalog.Definitions[id]
	name := d.Name
	if id == skills.BasicAttack {
		name = localization.Text("ui.app.basic_atk_9bb035dc")
	}
	if id == skills.Defense {
		name = localization.Text("ui.app.defense_6d3b491c")
	}
	if b.Target != 0 {
		name += localization.Text("ui.app.pet_6cb01d5c")
	}
	c.loadPictures(d.IconName())
	return d.IconName(), string(clientassets.Big5Text(name))
}
func (c *Client) assignBinding(page, slot byte, b hud.Binding) bool {
	if c.World == nil || c.UI.Modal != nil || page < 1 || page > protocol.HotbarPages || slot < 1 || slot > protocol.HotbarSlotsPerPage || !b.Valid() {
		return false
	}
	if b.ID != 0 {
		p := []byte{protocol.CommandHotbar, protocol.HotbarAssign, b.Kind}
		p = binary.LittleEndian.AppendUint16(p, b.ID)
		p = append(p, page, slot)
		if err := c.Net.Send(p); err != nil {
			c.Chat.Notice(err.Error())
			return false
		}
	}
	c.HotKeys.Bindings[page][slot] = b
	c.HotKeys.Refresh()
	c.saveHotbar()
	return true
}
func (c *Client) saveHotbar() {
	if c.hotbar.path != "" {
		if err := c.HotKeys.Bindings.Save(c.hotbar.path); err != nil {
			c.Chat.Notice(localization.UI("ui.app.could_not_save_hotbar_da8f4ed8") + err.Error())
		}
	}
}
func (c *Client) dropHotbar(x, y int) bool {
	d := c.hotbar.drag
	c.hotbar.drag = nil
	if d == nil || max(abs(x-d.start.X), abs(y-d.start.Y)) < hotbarDragDistance {
		return false
	}
	if c.UI.Modal != nil || c.sceneFrozen() {
		return true
	}
	slot := c.HotKeys.SlotAt(x, y)
	if slot != 0 && c.assignBinding(c.HotKeys.Page, slot, d.binding) && d.sourceSlot != 0 && (d.sourceSlot != slot || d.sourcePage != c.HotKeys.Page) {
		c.assignBinding(d.sourcePage, d.sourceSlot, hud.Binding{})
	}
	return true
}
func (c *Client) drawHotbarDrag() {
	d := c.hotbar.drag
	if d == nil || !c.Skills.Visible && d.sourceSlot == 0 || c.sceneFrozen() || c.UI.Modal != nil {
		return
	}
	if max(abs(c.Input.X-d.start.X), abs(c.Input.Y-d.start.Y)) < hotbarDragDistance {
		return
	}
	icon, _ := c.describeBinding(d.binding)
	c.Pics.Draw(c.Screen, c.Pics.Find(icon), c.Input.X-12, c.Input.Y-12, true)
}
func (c *Client) HotbarKey(key uint16, shift byte) bool {
	if key == escapeKey {
		if c.battle.item != nil {
			c.battle.item = nil
			return true
		}
		if c.battle.binding != nil {
			c.battle.binding = nil
			return true
		}
		if c.hotbar.drag != nil {
			c.hotbar.drag = nil
			return true
		}
		if c.hotbar.target != nil {
			c.closeHotbarTarget()
			return true
		}
	}
	if key < hotbarFirstKey || key > hotbarLastKey || shift != 0 {
		return false
	}
	if c.World == nil || !c.G.InGame || c.sceneFrozen() || c.UI.Modal != nil {
		return true
	}
	c.useBinding(c.HotKeys.Bindings[c.HotKeys.Page][key-hotbarFirstKey+1])
	return true
}
func (c *Client) hotbarPacket(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandHotbar || p[1] != protocol.HotbarAssign {
		return false
	}
	if (len(p)-2)%hotbarNativeRecordBytes != 0 {
		if c.Unhandled != nil {
			c.Unhandled(p)
		}
		return true
	}
	next := c.HotKeys.Bindings
	for at := 2; at < len(p); at += hotbarNativeRecordBytes {
		b := hud.Binding{Kind: p[at], ID: binary.LittleEndian.Uint16(p[at+1:])}
		page, slot := p[at+3], p[at+4]
		if b.ID == 0 {
			b = hud.Binding{}
		}
		if !b.Valid() || page < 1 || page > protocol.HotbarPages || slot < 1 || slot > protocol.HotbarSlotsPerPage {
			if c.Unhandled != nil {
				c.Unhandled(p)
			}
			return true
		}
		next[page][slot] = b
	}
	c.HotKeys.Bindings = next
	c.HotKeys.Refresh()
	c.saveHotbar()
	return true
}
func (c *Client) bindingLearned(b hud.Binding) bool {
	if b.Target != 0 {
		p := c.InventoryState.Pets[b.Target-1]
		if p.ID != b.Pet || p.ID == 0 {
			return false
		}
		if b.ID == skills.BasicAttack || b.ID == skills.Defense {
			return true
		}
		for _, s := range p.Skills {
			if s.ID == b.ID && s.Grade > 0 {
				return true
			}
		}
		return false
	}
	return b.ID == skills.BasicAttack || b.ID == skills.Defense || c.SkillState.Learned[b.ID].Grade > 0
}
func (c *Client) useBinding(b hud.Binding) {
	if b.ID == 0 || !b.Valid() || c.World == nil || !c.mapReady || c.sceneFrozen() || c.UI.Modal != nil {
		return
	}
	if b.Kind == protocol.HotbarItem {
		for slot, item := range c.InventoryState.Bag {
			if item.ID == b.ID && !item.Empty() && !item.Locked {
				if c.battle.state.Active {
					c.selectBattleItem(slot+1, false)
					return
				}
				c.Inventory.UseSelected(byte(slot+1), 1, b.Target, item)
				return
			}
		}
		c.Chat.Notice(localization.UI("ui.app.no_usable_item_for_this_shortcut_c6fe0491"))
		return
	}
	if !c.bindingLearned(b) {
		c.Chat.Notice(localization.UI("ui.app.this_skill_is_no_longer_available_to_the_4133b998"))
		return
	}
	battle := &c.remote.battle
	if !battle.active {
		c.Chat.Notice(localization.UI("ui.app.use_this_skill_during_battle_419e8ca8"))
		return
	}
	if !battle.ready || c.battleAnimating() {
		c.Chat.Notice(localization.UI("ui.app.wait_for_your_battle_turn_b4217e61"))
		return
	}
	if c.remote.active && c.remote.options.AutoFight {
		c.Chat.Notice(localization.UI("ui.app.stop_remote_auto_fight_before_choosing_a_9313d70e"))
		return
	}
	actor, ok := c.hotbarActor(b)
	if !ok {
		c.Chat.Notice(localization.UI("ui.app.the_selected_character_is_not_available__eef4e88b"))
		return
	}
	if c.hotbar.submitted[[2]byte{actor.x, actor.y}] {
		c.Chat.Notice(localization.UI("ui.app.an_action_has_already_been_selected_for__35b109ac"))
		return
	}
	id := c.bindingSkillID(b)
	if c.SkillState.Catalog.Definitions[id].SP > actor.sp {
		c.Chat.Notice(localization.UI("ui.app.not_enough_sp_5ce394fe"))
		return
	}
	if b.ID == skills.Defense {
		c.submitHotbar(b, actor)
		return
	}
	if c.battle.state.Active {
		c.battle.item = nil
		c.battle.selected = clientbattle.Cell{X: actor.x, Y: actor.y}
		c.battle.binding = &b
		c.Skills.Hide()
		c.BattleSkills.Hide()
		return
	}
	c.openHotbarTargets(b, actor)
}
func (c *Client) hotbarActor(b hud.Binding) (remoteFighter, bool) {
	for _, f := range c.remote.battle.fighters {
		if f.hp == 0 {
			continue
		}
		if native := c.battle.state.At(clientbattle.Cell{X: f.x, Y: f.y}); native != nil && (native.Removed || native.Waiting) {
			continue
		}
		if b.Target == 0 && f.kind == remoteFighterPlayer && f.id == c.World.Player.ID {
			return f, true
		}
		if b.Target != 0 && f.kind == remoteFighterPet && f.owner == c.World.Player.ID && f.id == game.BroadcastID(uint32(b.Pet)) {
			return f, true
		}
	}
	return remoteFighter{}, false
}
func (c *Client) closeHotbarTarget() {
	f := c.hotbar.target
	if f == nil {
		return
	}
	c.hotbar.target = nil
	if c.UI.Modal == f {
		c.UI.Modal = nil
	}
	f.Hide()
	c.UI.Remove(f)
}
func (c *Client) openHotbarTargets(b hud.Binding, actor remoteFighter) {
	c.closeHotbarTarget()
	f := seui.NewForm(c.Env)
	c.hotbar.target = f
	f.Dockable = false
	var targets []remoteFighter
	for _, t := range c.remote.battle.fighters {
		if c.hotbarTargetAllowed(b, actor, t) {
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		c.hotbar.target = nil
		c.Chat.Notice(localization.UI("ui.app.no_available_target_d7cdd7f3"))
		return
	}
	turn := c.hotbar.turn
	height := 80 + len(targets)*hotbarTargetRow
	f.Init("panel15", (ScreenWidth-hotbarTargetWidth)/2, 50, 50, 0, 0, true, height, hotbarTargetWidth, 60)
	f.SetMargins(15, 15, 15, 15)
	text := seui.NewEditor(c.Env, f)
	text.Init("", 18, 0, 0, 0, 0, false, 20, 265, 22)
	text.ReadOnly = true
	text.Color = hotbarTargetInk
	text.BindLanguage("text", func() { text.SetText([]byte(localization.UI("ui.app.choose_a_skill_target_204cef01"))) })
	for i, t := range targets {
		target := t
		button := seui.NewButton(c.Env, f)
		button.Init("btn_module_1", 18, 20, 56, 0, 0, true, 20, 264, 48+i*hotbarTargetRow)
		button.Color = hotbarTargetInk
		button.TextStyle = 0
		button.BindLanguage("caption", func() {
			name := fmt.Sprintf("%d,%d", target.x, target.y)
			if target.kind == remoteFighterPlayer && target.id == c.World.Player.ID {
				name = string(c.World.Player.Name)
			} else if template, ok := c.npcTemplates[target.id]; ok {
				name = string(clientassets.Big5Text(template.Name))
			}
			side := localization.UI("ui.app.ally_4685f718")
			if target.side != actor.side {
				side = localization.UI("ui.app.enemy_0021d5fe")
			}
			button.SetCaption([]byte(fmt.Sprintf(localization.UI("ui.app.s_s_d_hp_0f1b5d25"), side, name, target.hp)))
		})
		button.OnClick = func() {
			c.closeHotbarTarget()
			if c.hotbar.turn == turn {
				c.submitHotbar(b, target)
			}
		}
	}
	cancel := seui.NewButton(c.Env, f)
	cancel.Init("btn_module_1", 122, 20, 56, 0, 0, true, 20, 56, height-27)
	cancel.Color = hotbarTargetInk
	cancel.TextStyle = 0
	cancel.BindLanguage("caption", func() { cancel.SetCaption([]byte(localization.UI("ui.app.cancel_19766ed6"))) })
	cancel.OnClick = c.closeHotbarTarget
	c.UI.Add(f)
	c.UI.Modal = f
	f.Show()
}
func (c *Client) submitHotbar(b hud.Binding, target remoteFighter) {
	actor, ok := c.hotbarActor(b)
	if !ok || c.battleAnimating() || c.remote.active && c.remote.options.AutoFight || !c.remote.battle.active || !c.remote.battle.ready || !c.bindingLearned(b) || c.hotbar.submitted[[2]byte{actor.x, actor.y}] {
		return
	}
	id := c.bindingSkillID(b)
	if c.SkillState.Catalog.Definitions[id].SP > actor.sp {
		c.Chat.Notice(localization.UI("ui.app.not_enough_sp_5ce394fe"))
		return
	}
	found := false
	for _, t := range c.remote.battle.fighters {
		if t.x == target.x && t.y == target.y && t.id == target.id && (t.hp > 0 || c.SkillState.Catalog.Definitions[id].EffectLayer == nativeReviveLayer) {
			target = t
			found = true
			break
		}
	}
	if native := c.battle.state.At(clientbattle.Cell{X: target.x, Y: target.y}); native != nil && (native.Removed || native.Waiting) {
		return
	}
	if !found || b.ID != skills.Defense && !c.hotbarTargetAllowed(b, actor, target) {
		return
	}
	sub := byte(remoteBattleAttack)
	if b.ID == skills.Defense {
		sub = hotbarDefendAction
	}
	p := []byte{protocol.CommandBattleAction, sub, actor.x, actor.y, target.x, target.y}
	p = binary.LittleEndian.AppendUint16(p, b.ID)
	if err := c.sendCombatChoice(p); err != nil {
		c.Chat.Notice(err.Error())
		return
	}
	if c.hotbar.submitted == nil {
		c.hotbar.submitted = map[[2]byte]bool{}
	}
	c.hotbar.submitted[[2]byte{actor.x, actor.y}] = true
	c.rememberBattleSubmission(actor, b.ID)
}

// Native target policy gates roster and scene selection. Synthetic definitions
// without raw Skill.dat provenance retain the documented layer compatibility.
func (c *Client) hotbarTargetAllowed(b hud.Binding, actor, target remoteFighter) bool {
	if f := c.battle.state.At(clientbattle.Cell{X: target.x, Y: target.y}); f != nil && (f.Removed || f.Waiting) {
		return false
	}
	d := c.SkillState.Catalog.Definitions[c.bindingSkillID(b)]
	if d.EffectLayer == nativeReviveLayer && target.hp != 0 || d.EffectLayer != nativeReviveLayer && target.hp == 0 {
		return false
	}
	if d.TargetKnown {
		return d.AllowsTarget(target.side == actor.side, target.x == actor.x && target.y == actor.y)
	}
	enemy := c.battleBindingTargetsEnemy(b)
	return (enemy && target.side != actor.side) || (!enemy && target.side == actor.side)
}

// Shared target-domain policy for manual shortcuts and fresh-target repeats.
func (c *Client) battleBindingTargetsEnemy(b hud.Binding) bool {
	d := c.SkillState.Catalog.Definitions[c.bindingSkillID(b)]
	if d.TargetKnown {
		return d.AllowsTarget(false, false)
	}
	return b.ID == skills.BasicAttack || d.EffectLayer == skills.Physical || d.EffectLayer == skills.Magical ||
		d.EffectLayer == nativeControlLayer || d.EffectLayer == nativeDebuffLayer || d.EffectLayer == nativeDebuffAlternateLayer
}

func (c *Client) bindingSkillID(b hud.Binding) uint16 {
	if b.Target == 0 && b.ID == game.StarterStuntClientID && c.World != nil {
		return game.StarterStunt(uint16(c.World.Player.Body), uint16(c.World.Player.Head))
	}
	return b.ID
}
