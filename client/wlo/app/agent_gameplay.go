package app

import (
	"encoding/json"
	"fmt"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/minigame"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/protocol"
)

func agentExtendedActions() []agentAction {
	return []agentAction{
		agentActionDef("selection.cancel", "unsubmitted field target selection; submitted actions remain unchanged", nil, func(c *Client, _ map[string]any) error { return agentRejected(c.cancelFieldSelection()) }),
		agentActionDef("collection.stop", "client-local vacuum collection", nil, func(c *Client, _ map[string]any) error { c.stopVacuum(); return nil }),
		agentActionDef("walk.stop", "loaded world", nil, func(c *Client, _ map[string]any) error {
			if c.World == nil {
				return agentRejected(false)
			}
			c.resetGroundHold()
			c.World.StopWalk()
			return nil
		}),
		agentActionDef("remote.configure", "loaded character; complete RemoteOptions object; persists per character", []AgentArgument{{Name: "options", Type: "json"}, agentBoolean("start")}, func(c *Client, a map[string]any) error {
			var o inventory.RemoteOptions
			if seui.DecodeAutomation(a["options"].(json.RawMessage), &o) != nil || !o.Valid() {
				return fmt.Errorf("invalid Remote options")
			}
			if a["start"].(bool) {
				return agentRejected(c.startRemote(o))
			}
			if c.World == nil || !c.mapReady || c.remote.active {
				return agentRejected(false)
			}
			if err := saveRemoteProfile(c.remoteProfilePath(), o); err != nil {
				return fmt.Errorf("could not save Remote settings")
			}
			c.remote.options = o
			c.Inventory.RestoreRemoteOptions(o)
			return nil
		}),
		agentActionDef("chat.send", "online; native channel/radio/party permissions apply", []AgentArgument{agentInteger("channel", 1, 6), agentInteger("target", 0, int(^uint32(0))), agentString("text", 1, 1024)}, func(c *Client, a map[string]any) error {
			if c.World == nil || !c.mapReady {
				return agentRejected(false)
			}
			channel := byte(a["channel"].(int))
			target := uint32(a["target"].(int))
			text, err := seui.EncodeText(a["text"].(string))
			if err != nil {
				return err
			}
			if c.ChatBar.Message.Limit && len(text) > c.ChatBar.Message.MaxLen {
				return fmt.Errorf("message exceeds client limit")
			}
			switch channel {
			case hud.InputWorld:
				if !c.hasRadio() || c.Stats.SP < radioMinSP {
					return fmt.Errorf("world chat requires radio and SP")
				}
			case hud.InputLocal:
			case hud.InputWhisper:
				if target == 0 || c.playerName(target) == nil {
					return fmt.Errorf("whisper target is unavailable")
				}
			case hud.InputTeam:
				if !c.inTeam() {
					return fmt.Errorf("not in a party")
				}
			case hud.InputGuild:
				if !c.inGuild() {
					return fmt.Errorf("not in a guild")
				}
			default:
				return fmt.Errorf("unsupported chat channel")
			}
			c.ChatBar.SelectChannel(channel)
			c.ChatBar.Target = target
			c.ChatBar.Message.SetText(text)
			c.sendChat()
			return nil
		}),
		agentActionDef("world.target", "visible entity ID; operation joinTeam,friend,trade,pkPlayer,pkNPC,joinBattle,spectate", []AgentArgument{agentString("operation", 1, 32), agentInteger("id", 1, int(^uint32(0)))}, func(c *Client, a map[string]any) error {
			return c.agentTarget(a["operation"].(string), uint32(a["id"].(int)))
		}),
		agentActionDef("battle.leaveSpectating", "currently spectating", nil, func(c *Client, _ map[string]any) error {
			if !c.battle.state.Spectating {
				return agentRejected(false)
			}
			c.leaveSpectatedBattle()
			return nil
		}),
		agentActionDef("session.logout", "loaded character; opens normal confirmation prompt", nil, func(c *Client, _ map[string]any) error {
			if !c.canOpenModule() || c.battle.state.Active || c.UI.Modal != nil {
				return agentRejected(false)
			}
			c.logoutConfirmation(false)
			return nil
		}),
		agentActionDef("session.reconnect", "connection-lost form is visible; returns to normal server selection", nil, func(c *Client, _ map[string]any) error {
			if c.Lost == nil || !c.Lost.Visible {
				return agentRejected(false)
			}
			c.lostPrev()
			return nil
		}),
	}
}

func (c *Client) agentTarget(operation string, id uint32) error {
	if c.World == nil || !c.mapReady || !c.Inventory.CanAct() || c.UI.Modal != nil || c.afk.active || c.afk.resync {
		return agentRejected(false)
	}
	peer := c.World.Peers[id]
	if operation == "pkNPC" {
		if id > 0xffff || !c.canTargetPK() {
			return agentRejected(false)
		}
		npc := c.World.NPCs[uint16(id)]
		if npc == nil || !npc.Visible() || npc.Info.Prop() || abs(npc.X-c.World.Player.X) > pkTargetDistance || abs(npc.Y-c.World.Player.Y) > pkTargetDistance {
			return fmt.Errorf("NPC challenge target unavailable")
		}
		return c.Net.Send(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateChallenge, protocol.BattleChallengeNPC}.U32(npc.Template).U16(npc.ClickID))
	}
	if peer == nil || id == c.World.Player.ID {
		return fmt.Errorf("player target is not on this map")
	}
	switch operation {
	case "joinTeam":
		c.setJoinTeamTarget(false)
		c.toggleJoinTeam()
		if !c.joinTeamTarget {
			return agentRejected(false)
		}
		c.setJoinTeamTarget(false)
		if members := c.World.TeamMembers(id); len(members) > 1 {
			id = members[0]
		}
		return c.Net.Send(protocol.Builder{protocol.CommandTeam, protocol.TeamRequest}.U32(id))
	case "friend":
		c.setFriendTarget(false)
		c.makeFriends()
		if !c.friendTarget {
			return agentRejected(false)
		}
		c.setFriendTarget(false)
		if _, ok := c.friend(id); ok {
			return fmt.Errorf("already friends")
		}
		return c.Net.Send(protocol.Builder{protocol.CommandFriends, protocol.FriendsRequest}.U32(id))
	case "trade":
		c.setTradeTarget(false)
		c.tradeItems()
		if !c.tradeTarget {
			return agentRejected(false)
		}
		c.setTradeTarget(false)
		if peer.EffectiveLevel() < tradeMinLevel || c.sport != nil || c.World.Player.Map == mapNoBusinessA || c.World.Player.Map == mapNoBusinessB {
			return agentRejected(false)
		}
		return c.Net.Send(protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(id))
	case "pkPlayer":
		if !c.canTargetPK() || c.World.Fighting(id) {
			return agentRejected(false)
		}
		for _, member := range c.TeamState.Members {
			if member == id {
				return fmt.Errorf("cannot challenge own team")
			}
		}
		if abs(peer.X-c.World.Player.X) > pkTargetDistance || abs(peer.Y-c.World.Player.Y) > pkTargetDistance {
			return fmt.Errorf("target too far away")
		}
		return c.Net.Send(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateChallenge, protocol.BattleChallengePlayer}.U32(id).U16(0))
	case "joinBattle", "spectate":
		if !c.canEnterBattle() || !c.World.Fighting(id) || abs(peer.X-c.World.Player.X) > battleEntryDistance || abs(peer.Y-c.World.Player.Y) > battleEntryDistance {
			return agentRejected(false)
		}
		mode := byte(protocol.BattleJoinPlayer)
		if operation == "spectate" {
			mode = protocol.BattleWatchPlayer
		}
		if members := c.World.TeamMembers(id); len(members) > 0 {
			id = members[0]
		}
		return c.Net.Send(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateChallenge, mode}.U32(id).U16(0))
	}
	return fmt.Errorf("unsupported target operation")
}

// AgentDetails contains nonvisual state not already represented by visible forms.
func (c *Client) AgentDetails() map[string]any {
	out := map[string]any{"mapReady": c.mapReady, "held": c.held, "afk": map[string]any{"active": c.afk.active, "resync": c.afk.resync}, "remote": map[string]any{"active": c.remote.active, "options": c.remote.options}, "party": c.TeamState, "autoBattle": c.battle.auto, "spectating": c.battle.state.Spectating, "serverConfirmed": false}
	out["mapLoading"] = c.mapLoading.active
	out["sceneFrozen"] = c.sceneFrozen()
	out["control"] = c.AgentHostState()
	if c.SkillState != nil {
		out["learnedSkills"] = c.SkillState.Learned
	}
	if c.SettingsState != nil {
		out["settings"] = c.SettingsState
	}
	if c.Social != nil {
		out["friends"] = c.Social.AutomationInfo().Value
	}
	out["context"] = c.agentContext()
	out["collection"] = map[string]any{"actor": c.vacuum.actor, "map": c.vacuum.mapID, "active": c.vacuum.started, "waiting": c.vacuum.waiting, "next": c.vacuum.next, "cooldown": c.vacuum.cooldown.Seconds(), "timers": c.eventTimers}
	out["events"] = c.AgentEvents()
	if c.HotKeys != nil {
		out["hotbar"] = map[string]any{"page": c.HotKeys.Page, "bindings": c.HotKeys.Bindings}
	}
	if c.Chat != nil {
		lines := []map[string]any{}
		for _, line := range c.Chat.Lines {
			lines = append(lines, map[string]any{"speaker": line.Speaker, "channel": line.Channel, "text": seui.DecodeText(line.Text)})
		}
		out["chat"] = lines
	}
	notices := []string{}
	for _, notice := range c.Notices.items {
		notices = append(notices, seui.DecodeText(notice.text))
	}
	out["notices"] = notices
	if c.Servers != nil {
		decode := func(items [][]byte) []string {
			out := make([]string, len(items))
			for i, b := range items {
				out[i] = seui.DecodeText(b)
			}
			return out
		}
		out["serverSelection"] = map[string]any{"visible": c.Servers.Visible, "region": c.Servers.RegionIdx, "server": c.Servers.ServerRow, "regions": decode(c.Servers.Regions.Items), "channels": decode(c.Servers.Servers.Items)}
	}
	if w := c.afkWorkspace; w != nil && w.agentDisplay != nil {
		d := w.agentDisplay
		out["display"] = map[string]any{"settings": d.settings, "draft": d.draft, "preview": d.preview, "error": d.error, "deadline": d.deadline}
	}
	out["housing"] = c.housing
	if c.Create != nil {
		out["creation"] = map[string]any{"visible": c.Create.Visible, "step": c.Create.Step, "selected": c.Create.Selected, "points": c.Create.Points, "stats": c.Create.Stats, "waiting": c.Create.Waiting}
	}
	if sp := c.sport; sp != nil {
		info := map[string]any{"session": sp.apiID, "kind": sp.kind, "running": sp.game.Running(), "done": sp.game.Done(), "canStart": sp.start.Visible}
		if g, ok := sp.game.(minigame.Automation); ok {
			info["state"] = seui.AutomationDescription(g.AutomationState())
		}
		out["minigame"] = info
	}
	return out
}
