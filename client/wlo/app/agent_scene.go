package app

import (
	"encoding/json"
	"fmt"
	"image"
	"wonderland-gonline/client/wlo/housing"
	"wonderland-gonline/client/wlo/minigame"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func agentSceneActions() []agentAction {
	return []agentAction{
		agentActionDef("housing.action", "loaded scene; furniture coordinates use grid cells", []AgentArgument{agentString("operation", 1, 24), {Name: "parameters", Type: "json"}}, func(c *Client, a map[string]any) error {
			return c.agentHousing(a["operation"].(string), a["parameters"].(json.RawMessage))
		}),
		agentActionDef("vehicle.board", "owned placed vehicle near character", []AgentArgument{agentInteger("slot", 1, game.BagSize)}, func(c *Client, a map[string]any) error {
			if c.World == nil || !c.Inventory.CanAct() || c.UI.Modal != nil {
				return agentRejected(false)
			}
			return agentRejected(c.requestPlacedVehicle(vehicleKey{c.World.Player.ID, byte(a["slot"].(int))}))
		}),
		agentActionDef("minigame.action", "current minigame session; semantic action from details.minigame.state.actions", []AgentArgument{agentString("session", 1, 128), agentString("action", 1, 32), {Name: "value", Type: "json"}}, func(c *Client, a map[string]any) error {
			sp := c.sport
			if sp == nil || sp.apiID != a["session"].(string) {
				return fmt.Errorf("minigame session expired")
			}
			action := a["action"].(string)
			if action == "start" || action == "leave" {
				if err := seui.ValidateAutomationSchema(a["value"].(json.RawMessage), seui.SchemaOf(nil)); err != nil {
					return err
				}
			}
			if action == "start" {
				if !sp.start.Visible {
					return agentRejected(false)
				}
				c.sportStartClick(sp, sportTagStart)
				return nil
			}
			if action == "leave" {
				if sp.talk != nil && sp.talk.Shown() {
					return fmt.Errorf("answer pending game prompt first")
				}
				if g, ok := sp.game.(*minigame.Mario); ok {
					g.Exit()
				} else {
					sp.game.End(false)
				}
				return nil
			}
			if !sp.game.Running() || sp.game.Done() || sp.talk != nil && sp.talk.Shown() {
				return agentRejected(false)
			}
			g, ok := sp.game.(minigame.Automation)
			if !ok {
				return fmt.Errorf("game does not expose semantic actions")
			}
			if err := seui.ValidateAutomationAction(g.AutomationState(), action, a["value"].(json.RawMessage)); err != nil {
				return err
			}
			return g.AutomationAction(action, a["value"].(json.RawMessage), c.Now())
		}),
	}
}

func (c *Client) agentHousing(action string, raw json.RawMessage) error {
	if c.World == nil || !c.Inventory.CanAct() || c.UI.Modal != nil || c.afk.active || c.afk.resync {
		return agentRejected(false)
	}
	var p struct {
		Owner uint32
		Index uint16
		Slot  int
		Item  uint16
		X, Y  int
	}
	if action != "exit" && action != "close" && seui.DecodeAutomation(raw, &p) != nil {
		return fmt.Errorf("invalid housing parameters")
	}
	switch action {
	case "enter":
		sign, ok := c.housing.Signs[p.Owner]
		if !ok {
			return fmt.Errorf("tent is not on this map")
		}
		c.pendingNPC = nil
		c.floorPendingPickup = 0
		c.resetGroundHold()
		c.tentPending = p.Owner
		if !c.enterNearbyTent() {
			cx, cy := c.World.Camera()
			c.walkTo(int(sign.X)-cx, int(sign.Y)-cy)
		}
		return nil
	case "exit":
		if !game.IsTentMap(c.World.Player.Map) || c.tentLeaving {
			return agentRejected(false)
		}
		exit := housing.Exit(c.tentKind)
		if abs(c.World.Player.X-exit.X) > tentExitReach || abs(c.World.Player.Y-exit.Y) > tentExitReach {
			return fmt.Errorf("walk to tent exit first")
		}
		c.World.StopWalk()
		c.resetGroundHold()
		c.tentLeaving = true
		return c.Net.Send([]byte{protocol.CommandTent, protocol.TentExit})
	case "close":
		if c.openTentItem == 0 {
			return fmt.Errorf("no owned tent is open")
		}
		c.Inventory.CloseTent(c.openTentItem)
		return nil
	case "place":
		if !game.IsTentMap(c.World.Player.Map) || c.housing.Owner != c.World.Player.ID || p.Slot < 1 || p.Slot > game.BagSize {
			return agentRejected(false)
		}
		it := c.InventoryState.Bag[p.Slot-1]
		if it.ID != p.Item || it.Empty() || it.Locked {
			return fmt.Errorf("furniture item changed")
		}
		kind := c.items[it.ID].Definition.Type
		if kind != game.ItemTypeTentFurniture && kind != game.ItemTypeAppliance && kind != game.ItemTypeTentDecoration {
			return fmt.Errorf("item is not furniture")
		}
		if !image.Pt(p.X, p.Y).In(housing.Room(c.tentKind)) {
			return fmt.Errorf("furniture cell is outside room")
		}
		return c.Net.Send(protocol.Builder{protocol.CommandTentFurniture, protocol.TentPlaceFurniture, byte(p.Slot)}.U32(uint32(p.X)).U32(uint32(p.Y)).U32(1).U8(c.housing.Floor))
	case "move", "floorSwitch":
		f, ok := c.housing.Furniture[p.Index]
		if !ok || f.Item != p.Item || f.Floor != c.housing.Floor {
			return fmt.Errorf("furniture changed or is not on this floor")
		}
		if action == "floorSwitch" {
			if !c.items[f.Item].Definition.TentFloorSwitch {
				return fmt.Errorf("not a floor switch")
			}
			return c.Net.Send(protocol.Builder{protocol.CommandTentFurniture, protocol.TentFloorSurface}.U16(p.Index))
		}
		if c.housing.Owner != c.World.Player.ID || p.X < 0 || p.Y < 0 {
			return agentRejected(false)
		}
		next := f
		next.X, next.Y = uint32(p.X), uint32(p.Y)
		if !tentFootprint(c.items[f.Item], next).In(housing.Room(c.tentKind)) {
			return fmt.Errorf("furniture does not fit room")
		}
		return c.Net.Send(protocol.Builder{protocol.CommandTentFurniture, protocol.TentMoveFurniture}.U16(p.Index).U32(next.X).U32(next.Y).U32(f.Z).U8(f.Rotation))
	}
	return fmt.Errorf("unsupported housing action")
}
