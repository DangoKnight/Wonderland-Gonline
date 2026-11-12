package app

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/protocol"
)

func (c *Client) agentContext() AgentContext {
	out := AgentContext{Turn: c.hotbar.turn}
	if c.World != nil {
		out.Character = c.World.Player.ID
		out.Map = c.World.Player.Map
	}
	return out
}

func (c *Client) AgentSession(id int) *Client {
	if id == 0 {
		if c.afkWorkspace != nil {
			return c.afkWorkspace.Current()
		}
		return c
	}
	if c.afkWorkspace != nil {
		for _, s := range c.afkWorkspace.Sessions {
			if s.ID == id {
				return s.Client
			}
		}
	}
	return nil
}
func (c *Client) agentSessionID() int {
	if c.afkWorkspace != nil {
		for _, s := range c.afkWorkspace.Sessions {
			if s.Client == c {
				return s.ID
			}
		}
	}
	return 0
}
func (c *Client) AgentSessions() []map[string]any {
	out := []map[string]any{}
	if c.afkWorkspace == nil {
		return out
	}
	for _, s := range c.afkWorkspace.Sessions {
		if err := s.Client.AgentAccessError(); err != nil {
			out = append(out, map[string]any{"id": s.ID, "error": err.Error()})
			continue
		}
		row := map[string]any{"id": s.ID, "title": s.Title, "active": s.Client == c.afkWorkspace.Current(), "afk": s.Client.afk.active}
		if s.Client.World != nil {
			row["player"] = s.Client.World.Player.ID
			row["map"] = s.Client.World.Player.Map
		}
		out = append(out, row)
	}
	return out
}

func agentWorkspaceActions() []agentAction {
	return []agentAction{
		agentActionDef("workspace.action", "same application workspace; add, switch, close, title, collapse", []AgentArgument{agentString("operation", 1, 24), {Name: "parameters", Type: "json"}}, func(c *Client, a map[string]any) error {
			w := c.afkWorkspace
			if w == nil {
				return fmt.Errorf("workspace unavailable")
			}
			for _, session := range w.Sessions {
				if session.Client.agentViewerOwns() {
					return fmt.Errorf("workspace changes are suspended during viewer intervention")
				}
			}
			action := a["operation"].(string)
			raw := a["parameters"].(json.RawMessage)
			if action == "add" {
				return w.Add()
			}
			if action == "collapse" {
				var collapsed bool
				if seui.DecodeAutomation(raw, &collapsed) != nil {
					return fmt.Errorf("boolean required")
				}
				w.Collapsed = collapsed
				return nil
			}
			var p struct {
				ID    int
				Title string
			}
			if seui.DecodeAutomation(raw, &p) != nil {
				return fmt.Errorf("invalid session parameters")
			}
			for i, s := range w.Sessions {
				if s.ID != p.ID {
					continue
				}
				switch action {
				case "switch":
					w.Switch(i)
				case "close":
					s.Client.Exit = true
				case "title":
					if len(p.Title) > 128 {
						return fmt.Errorf("session title too long")
					}
					s.Title = p.Title
				default:
					return fmt.Errorf("unsupported workspace action")
				}
				return nil
			}
			return fmt.Errorf("session does not belong to this workspace")
		}),
		agentActionDef("display.preview", "workspace display controls; preview settings before confirming", []AgentArgument{{Name: "settings", Type: "json"}}, func(c *Client, a map[string]any) error {
			if c.afkWorkspace == nil || c.afkWorkspace.agentDisplay == nil {
				return fmt.Errorf("display controls unavailable")
			}
			var settings DisplaySettings
			if seui.DecodeAutomation(a["settings"].(json.RawMessage), &settings) != nil {
				return fmt.Errorf("invalid display settings")
			}
			if err := settings.validate(); err != nil {
				return err
			}
			d := c.afkWorkspace.agentDisplay
			d.draft = settings
			d.applyDraft(c.Now())
			if d.error != "" {
				return fmt.Errorf("display settings could not be applied")
			}
			return nil
		}),
		agentActionDef("display.confirm", "active display preview", nil, func(c *Client, _ map[string]any) error {
			if c.afkWorkspace == nil || c.afkWorkspace.agentDisplay == nil {
				return agentRejected(false)
			}
			d := c.afkWorkspace.agentDisplay
			if !d.preview {
				return agentRejected(false)
			}
			d.keep()
			if d.error != "" {
				return fmt.Errorf("display settings could not be saved")
			}
			return nil
		}),
		agentActionDef("display.revert", "workspace display controls", nil, func(c *Client, _ map[string]any) error {
			if c.afkWorkspace == nil || c.afkWorkspace.agentDisplay == nil {
				return agentRejected(false)
			}
			c.afkWorkspace.agentDisplay.revert()
			return nil
		}),
		agentActionDef("hotbar.assign", "loaded character; validated shortcut binding", []AgentArgument{agentInteger("page", 1, protocol.HotbarPages), agentInteger("slot", 1, protocol.HotbarSlotsPerPage), {Name: "binding", Type: "json"}}, func(c *Client, a map[string]any) error {
			var b hud.Binding
			if seui.DecodeAutomation(a["binding"].(json.RawMessage), &b) != nil || !b.Valid() {
				return fmt.Errorf("invalid hotbar binding")
			}
			return agentRejected(c.assignBinding(byte(a["page"].(int)), byte(a["slot"].(int)), b))
		}),
		agentActionDef("hotbar.use", "loaded character; normal shortcut activation", []AgentArgument{agentInteger("page", 1, protocol.HotbarPages), agentInteger("slot", 1, protocol.HotbarSlotsPerPage)}, func(c *Client, a map[string]any) error {
			if c.World == nil || c.UI.Modal != nil {
				return agentRejected(false)
			}
			b := c.HotKeys.Bindings[a["page"].(int)][a["slot"].(int)]
			if b.ID == 0 {
				return fmt.Errorf("shortcut is empty")
			}
			c.useBinding(b)
			return nil
		}),
	}
}
