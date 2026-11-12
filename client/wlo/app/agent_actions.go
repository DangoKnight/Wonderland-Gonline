package app

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/hud"

	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func agentInteger(name string, min, max int) AgentArgument {
	return AgentArgument{Name: name, Type: "integer", Min: min, Max: max}
}
func agentString(name string, min, max int) AgentArgument {
	return AgentArgument{Name: name, Type: "string", Min: min, Max: max}
}
func agentBoolean(name string) AgentArgument { return AgentArgument{Name: name, Type: "boolean"} }
func agentActionDef(name, prerequisite string, args []AgentArgument, run func(*Client, map[string]any) error) agentAction {
	return agentAction{AgentCommandDefinition{Name: name, Arguments: args, Prerequisite: prerequisite}, run}
}
func agentRejected(ok bool) error {
	if !ok {
		return fmt.Errorf("action rejected by current client state")
	}
	return nil
}

var agentActions []agentAction

func init() { agentActions = buildAgentActions() }

func buildAgentActions() []agentAction {
	world := "loaded world; normal gameplay eligibility checks apply"
	decision := "battle decision stage; actor, target, skill and SP must be eligible"
	xy := []AgentArgument{agentInteger("x", 0, 65535), agentInteger("y", 0, 65535)}
	target := []AgentArgument{agentInteger("targetX", 0, 255), agentInteger("targetY", 0, 255)}
	slot := []AgentArgument{agentInteger("slot", 1, game.BagSize)}
	actions := []agentAction{
		agentActionDef("ui.invoke", "visible control; current instance-scoped handle and revision", []AgentArgument{agentString("id", 1, 128), agentString("revision", 64, 64), agentString("action", 1, 32), {Name: "value", Type: "json"}}, func(c *Client, a map[string]any) error {
			return c.agentInvokeControl(a["id"].(string), a["revision"].(string), a["action"].(string), a["value"].(json.RawMessage))
		}),
		agentActionDef("server.connect", "server selection is open; zero-based region and channel indices", []AgentArgument{agentInteger("region", 0, 255), agentInteger("channel", 0, 255)}, func(c *Client, a map[string]any) error {
			if c.Servers == nil || !c.Servers.Visible || a["region"].(int) >= c.Servers.Regions.Count() {
				return agentRejected(false)
			}
			c.Servers.SelectRegion(byte(a["region"].(int)))
			channel := a["channel"].(int)
			if channel >= len(c.Servers.Addresses) {
				return fmt.Errorf("channel unavailable")
			}
			c.Servers.Connect(channel)
			return nil
		}),
		agentActionDef("login.submit", "login form is open; credentials are never retained in command history", []AgentArgument{{Name: "account", Type: "string", Min: 1, Max: 10, Sensitive: true}, {Name: "password", Type: "string", Min: 1, Max: 10, Sensitive: true}}, func(c *Client, a map[string]any) error {
			if c.Login == nil || !c.Login.Visible {
				return agentRejected(false)
			}
			c.Login.Account.SetText([]byte(a["account"].(string)))
			c.Login.Password.SetText([]byte(a["password"].(string)))
			c.Login.Submit()
			return agentRejected(!c.Login.Visible)
		}),
		agentActionDef("character.select", "character selection is open and slot is occupied", []AgentArgument{agentInteger("slot", 1, 2)}, func(c *Client, a map[string]any) error {
			n := a["slot"].(int)
			if c.Chars == nil || !c.Chars.Visible || c.Chars.Slots[n].Level == 0 {
				return agentRejected(false)
			}
			b := c.Chars.Enter[n-1].Base()
			if b.OnClickTag == nil {
				return agentRejected(false)
			}
			b.OnClickTag(b.Tag)
			return nil
		}),
		agentActionDef("walk.to", world, xy, func(c *Client, a map[string]any) error {
			return agentRejected(c.AgentWalkWorld(a["x"].(int), a["y"].(int)))
		}),
		agentActionDef("world.interactNPC", world, []AgentArgument{agentInteger("id", 1, 65535)}, func(c *Client, a map[string]any) error {
			return agentRejected(c.AgentInteractNPC(uint16(a["id"].(int))))
		}),
		agentActionDef("world.pickup", world, []AgentArgument{agentInteger("slot", 1, 65535)}, func(c *Client, a map[string]any) error {
			return agentRejected(c.AgentPickupFloorSlot(uint16(a["slot"].(int))))
		}),
		agentActionDef("dialogue.advance", "dialogue is open and is not asking a question", nil, func(c *Client, _ map[string]any) error {
			if c.Talk == nil || !c.Talk.Shown() || c.Talk.Choosing() {
				return agentRejected(false)
			}
			c.advanceTalk()
			return nil
		}),
		agentActionDef("dialogue.answer", "dialogue question is open; zero-based option index", []AgentArgument{agentInteger("index", 0, 254)}, func(c *Client, a map[string]any) error { return agentRejected(c.AgentAnswer(a["index"].(int))) }),
		agentActionDef("afk.set", "Gonline AFK negotiated; interactive forms must be closed", []AgentArgument{agentBoolean("enabled")}, func(c *Client, a map[string]any) error { return c.SetAFK(a["enabled"].(bool)) }),
		agentActionDef("battle.attack", decision, target, func(c *Client, a map[string]any) error {
			return agentRejected(c.AgentBattleAttack(a["targetX"].(int), a["targetY"].(int)))
		}),
		agentActionDef("battle.defend", decision, nil, func(c *Client, _ map[string]any) error { return agentRejected(c.AgentBattleDefend()) }),
		agentActionDef("battle.flee", decision, nil, func(c *Client, _ map[string]any) error { return agentRejected(c.AgentBattleFlee()) }),
		agentActionDef("battle.capture", decision, target, func(c *Client, a map[string]any) error {
			if !c.battle.state.Active || c.battleAnimating() {
				return agentRejected(false)
			}
			actor, ok := c.battleActor()
			if !ok {
				return agentRejected(false)
			}
			victim, ok := c.agentFighterAt(a["targetX"].(int), a["targetY"].(int))
			if !ok {
				return agentRejected(false)
			}
			c.submitBattleSpecial(actor, victim, battleCatchSkill, protocol.BattleActionAttackRequest)
			return agentRejected(c.hotbar.submitted[[2]byte{actor.x, actor.y}])
		}),
		agentActionDef("battle.skill", decision, append([]AgentArgument{agentInteger("skill", 1, 65535)}, target...), func(c *Client, a map[string]any) error {
			return agentRejected(c.AgentBattleSkill(uint16(a["skill"].(int)), a["targetX"].(int), a["targetY"].(int)))
		}),
		agentActionDef("battle.item", decision, append(append(append([]AgentArgument{}, slot...), agentInteger("item", 1, 65535)), target...), func(c *Client, a map[string]any) error {
			if c.InventoryState == nil || int(c.InventoryState.Bag[a["slot"].(int)-1].ID) != a["item"].(int) {
				return fmt.Errorf("battle item changed")
			}
			return agentRejected(c.AgentBattleItem(a["slot"].(int), a["targetX"].(int), a["targetY"].(int)))
		}),
		agentActionDef("battle.auto", "battle is active", []AgentArgument{agentBoolean("enabled")}, func(c *Client, a map[string]any) error {
			if !c.battle.state.Active {
				return agentRejected(false)
			}
			c.AgentBattleAuto(a["enabled"].(bool))
			return nil
		}),
		agentActionDef("inventory.use", world+"; outside battle; pet 0 selects player", []AgentArgument{agentInteger("slot", 1, game.BagSize), agentInteger("pet", 0, game.MaxPets), agentInteger("item", 1, 65535)}, func(c *Client, a map[string]any) error {
			if !c.canOpenModule() || c.battle.state.Active || c.afk.active || c.afk.resync || c.UI.Modal != nil {
				return agentRejected(false)
			}
			slot, pet := a["slot"].(int), a["pet"].(int)
			item := c.InventoryState.Bag[slot-1]
			if item.Empty() || item.Locked || int(item.ID) != a["item"].(int) || pet > 0 && c.InventoryState.Pets[pet-1].ID == 0 {
				return agentRejected(false)
			}
			c.Inventory.Selected = byte(pet)
			c.Inventory.Show()
			c.Inventory.Use(slot, false)
			return nil
		}),
		agentActionDef("inventory.unequip", world+"; player equipment", []AgentArgument{agentInteger("slot", 1, inventory.EquipmentSlots), agentInteger("item", 1, 65535)}, func(c *Client, a map[string]any) error {
			if !c.canOpenModule() || c.battle.state.Active || c.afk.active || c.afk.resync || c.UI.Modal != nil {
				return agentRejected(false)
			}
			slot := a["slot"].(int)
			if c.InventoryState.Equipment[slot-1].Empty() || int(c.InventoryState.Equipment[slot-1].ID) != a["item"].(int) || c.InventoryState.FirstFree() == 0 {
				return agentRejected(false)
			}
			c.Inventory.Selected = 0
			c.Inventory.Show()
			c.Inventory.Use(slot, true)
			return nil
		}),
		agentActionDef("remote.start", world+"; an unlocked Remote must be owned", []AgentArgument{agentBoolean("autoFight"), agentBoolean("autoMove"), agentBoolean("autoSupply")}, func(c *Client, a map[string]any) error {
			o := c.remote.options
			if !o.Valid() {
				o = inventory.RemoteOptions{SupplyFirst: 1, SupplyLast: game.BagSize}
			}
			o.AutoFight, o.AutoMove, o.AutoSupply = a["autoFight"].(bool), a["autoMove"].(bool), a["autoSupply"].(bool)
			return agentRejected(c.startRemote(o))
		}),
		agentActionDef("remote.stop", "client-local automation", nil, func(c *Client, _ map[string]any) error { c.stopRemote(); return nil }),
		agentActionDef("pose.set", world, []AgentArgument{agentInteger("cell", 0, 255)}, func(c *Client, a map[string]any) error { return agentRejected(c.AgentPose(a["cell"].(int))) }),
	}
	for _, name := range []string{"menu.open", "menu.close"} {
		open := name == "menu.open"
		actions = append(actions, agentActionDef(name, "named player module; same toolbar validation as manual use", []AgentArgument{agentString("module", 1, 32)}, func(c *Client, a map[string]any) error { return c.agentMenu(a["module"].(string), open) }))
	}
	actions = append(append(append(actions, agentExtendedActions()...), agentSceneActions()...), agentWorkspaceActions()...)
	actions = append(actions, agentNotificationActions()...)
	for i := range actions {
		actions[i].definition.Variants = agentOperationSchemas(actions[i].definition.Name)
		var sample any
		switch actions[i].definition.Name {
		case "remote.configure":
			sample = inventory.RemoteOptions{}
		case "display.preview":
			sample = DisplaySettings{}
		case "hotbar.assign":
			sample = hud.Binding{}
		}
		if sample != nil {
			schema := seui.SchemaOf(sample)
			for j := range actions[i].definition.Arguments {
				if actions[i].definition.Arguments[j].Type == "json" {
					actions[i].definition.Arguments[j].Schema = &schema
				}
			}
		}
	}
	return actions
}

func (c *Client) agentModule(name string) (seui.Control, int) {
	switch name {
	case "inventory":
		return c.Inventory, mainInventoryButton
	case "skills":
		return c.Skills, mainSkillsButton
	case "compound":
		return c.Compound, mainCompoundButton
	case "team":
		return c.Team, mainTeamButton
	case "social":
		return c.Social, mainSocialButton
	case "settings":
		return c.Settings, mainSettingsButton
	case "mall":
		return c.Mall, 0
	case "worldPK":
		return c.WorldPK, 0
	case "notebook":
		return c.Notebook, 0
	case "mailbox":
		return c.Mailbox, 0
	case "business":
		return c.Business, 0
	case "luckyDraw":
		return c.LuckyDraw, 0
	}
	return nil, 0
}
func (c *Client) agentMenu(name string, open bool) error {
	form, button := c.agentModule(name)
	if form == nil {
		return fmt.Errorf("module unavailable")
	}
	if c.afk.active || c.afk.resync || c.UI.Modal != nil {
		return fmt.Errorf("module blocked by current interaction")
	}
	if form.Base().Visible == open {
		return nil
	}
	if button == 0 {
		return c.agentExtraMenu(name, open)
	}
	c.MainButtons.Buttons[button].OnClick()
	return agentRejected(form.Base().Visible == open)
}

type AgentFormState struct {
	Name     string              `json:"name"`
	Modal    bool                `json:"modal"`
	Buttons  []AgentButtonState  `json:"buttons"`
	Controls []AgentControlState `json:"controls"`
}
type AgentButtonState struct {
	Name string `json:"name"`
	Text string `json:"text"`
}
