package app

import (
	"encoding/binary"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	mainInventoryButton      = 1
	inventoryWarningDuration = 2 * time.Second
)

func (c *Client) initInventory() {
	c.InventoryState = &inventory.State{Items: c.items}
	c.Inventory = inventory.NewForm(c.Env, c.InventoryState, c.Stats)
	c.Inventory.Send = func(p []byte) {
		if err := c.Net.Send(p); err != nil {
			c.Chat.Notice(err.Error())
		}
	}
	petSprites := make(map[uint16]*role.NPC)
	c.Inventory.DrawPet = func(slot byte, x, y, action int) {
		if c.lib == nil {
			return
		}
		id := c.InventoryState.Pets[slot-1].ID
		sprite := petSprites[id]
		if sprite == nil {
			if c.npcTemplates == nil {
				c.npcTemplates, _ = world.NPCTemplates(c.Assets)
			}
			template, ok := c.npcTemplates[uint32(id)]
			if !ok {
				return
			}
			sprite = role.NewNPC(c.lib, template.Look, template.Colors)
			sprite.Now = func() time.Time { return c.Now() }
			petSprites[id] = sprite
		}
		sprite.Draw(c.Screen, x, y, action)
	}
	c.Inventory.CanAct = func() bool {
		return c.World != nil && c.mapReady && c.movie == nil && c.sport == nil && !c.held && !c.event.active && !c.sceneFrozen()
	}
	c.Inventory.Now = func() time.Time { return c.Now() }
	c.Inventory.Notice = c.Chat.Notice
	c.Inventory.StartRemote = c.startRemote
	c.Inventory.StopRemote = c.stopRemote
	c.Inventory.Warning = func(message string) {
		c.Notices.Show([]byte(message), inventoryWarningDuration, c.Now())
	}
	c.UI.Add(c.Inventory)
	c.MainButtons.Buttons[mainInventoryButton].OnClick = func() {
		if c.Inventory.Visible {
			c.Inventory.Hide()
		} else if c.Inventory.CanAct() {
			c.Inventory.Show()
		}
	}
}

func (c *Client) resetInventory(p world.Player) {
	if c.Settings != nil {
		c.Settings.Hide()
		c.SettingsState.Reset()
		c.applyLocalSettings()
	}
	c.Inventory.Hide()
	c.Compound.Reset()
	c.Team.Reset()
	c.TeamState.Reset(p.ID)
	c.teamAppearances = nil
	c.setJoinTeamTarget(false)
	c.Inventory.AllocationReply()
	c.Inventory.ResetUse()
	c.Inventory.ResetRemote()
	c.resetRemoteRuntime()
	c.Inventory.Select(0)
	c.InventoryState.Reset(p.Items)
	if c.Skills != nil {
		c.SkillState.Reset()
		c.Skills.Reset()
	}
	*c.Stats = world.Stats{Formula: c.Stats.Formula}
	c.Inventory.PlayerName = append([]byte(nil), p.Name...)
	if c.lib != nil {
		preview := role.NewHuman(c.lib, c.items)
		preview.Now = func() time.Time { return c.Now() }
		c.Inventory.Preview = preview
	}
	c.refreshEquipment()
	c.loadHotbar()
}

func (c *Client) refreshEquipment() {
	if c.World == nil {
		return
	}
	p := &c.World.Player
	slot := &login.CharacterSlot{Body: p.Body, Head: p.Head, Color1: p.Color1, Color2: p.Color2, Name: p.Name, Direction: p.Direction}
	p.Items = nil
	for i, it := range c.InventoryState.Equipment {
		slot.Equipment[i+1] = it.ID
		if !it.Empty() {
			p.Items = append(p.Items, it.ID)
		}
	}
	if c.World.Body != nil {
		c.World.Body.SetCharacter(slot)
	}
	if c.Inventory.Preview != nil {
		c.Inventory.Preview.SetCharacter(slot)
	}
}

func (c *Client) inventoryPacket(p []byte) {
	if c.World == nil || c.InventoryState == nil || len(p) < 2 {
		return
	}
	before := c.InventoryState.Equipment
	if handled, valid := c.InventoryState.Apply(p); handled {
		if !valid {
			if c.Unhandled != nil {
				c.Unhandled(p)
			}
			return
		}
		if p[1] == protocol.InventoryCompoundResult {
			c.Compound.QueueResult(p[2])
		}
		for _, it := range c.InventoryState.Bag {
			if icon := c.items[it.ID].Icon; !it.Empty() && icon != 0 {
				c.loadPictures(strconv.Itoa(int(icon)))
			}
		}
		for _, it := range c.InventoryState.Equipment {
			if icon := c.items[it.ID].Icon; !it.Empty() && icon != 0 {
				c.loadPictures(strconv.Itoa(int(icon)))
			}
		}
		for _, pet := range c.InventoryState.Pets {
			for _, it := range pet.Equipment {
				if icon := c.items[it.ID].Icon; !it.Empty() && icon != 0 {
					c.loadPictures(strconv.Itoa(int(icon)))
				}
			}
		}
		if before != c.InventoryState.Equipment {
			c.refreshEquipment()
		}
		return
	}
	switch p[1] {
	case protocol.InventoryCompoundSuccess:
		if len(p) == 6 && p[4] != 0 && p[5] >= 1 && p[5] <= game.BagSize {
			c.Compound.Result(binary.LittleEndian.Uint16(p[2:]), p[4], p[5])
		} else if c.Unhandled != nil {
			c.Unhandled(p)
		}
	case protocol.InventoryFishingStopped:
		// Native AC23:122 clears rod presentation; it is not a synthesis effect.
		if len(p) != 6 && c.Unhandled != nil {
			c.Unhandled(p)
		}
	case protocol.InventoryPotentialPillResult:
		if !c.Inventory.PotentialReply(p) && c.Unhandled != nil {
			c.Unhandled(p)
		}
	case protocol.InventoryItemUse:
		c.remoteInventoryReceipt(p)
		// AC23:15 acknowledges server-authoritative consumable use.

	case protocol.InventoryWireCode212:
		if len(p) == 7 && p[2] == 255 {
			c.Inventory.ConfirmDestroy(p[3], binary.LittleEndian.Uint16(p[4:]), p[6])
		}
	case protocol.InventoryMessage:
		if len(p) >= 4 {
			n := int(binary.LittleEndian.Uint16(p[2:]))
			if len(p) == 4+n {
				c.Chat.Notice(string(p[4:]))
			}
		}
	default:
		if c.Unhandled != nil {
			c.Unhandled(p)
		}
	}
}

// InventoryKey closes the inventory even when a child slot has focus.
func (c *Client) InventoryKey(key uint16) bool {
	if key != 0x1b || c.Inventory == nil || !c.Inventory.Visible {
		return false
	}
	c.Inventory.Hide()
	return true
}
