package app

import (
	"encoding/binary"
	"strconv"
	"time"

	"wonderland-go/client/wlo/inventory"
	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/role"
	"wonderland-go/client/wlo/world"
	"wonderland-go/internal/protocol"
)

const mainInventoryButton = 1

func (c *Client) initInventory() {
	c.InventoryState = &inventory.State{Items: c.items}
	c.Inventory = inventory.NewForm(c.Env, c.InventoryState, c.Stats)
	c.Inventory.Send = func(p []byte) {
		if err := c.Net.Send(p); err != nil {
			c.Chat.Notice(err.Error())
		}
	}
	c.Inventory.CanAct = func() bool {
		return c.World != nil && c.mapReady && c.movie == nil && c.sport == nil && !c.held && !c.event.active && !c.sceneFrozen()
	}
	c.Inventory.Now = func() time.Time { return c.Now() }
	c.Inventory.Notice = c.Chat.Notice
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
	c.Inventory.AllocationReply()
	c.InventoryState.Reset(p.Items)
	*c.Stats = world.Stats{Formula: c.Stats.Formula}
	c.Inventory.PlayerName = append([]byte(nil), p.Name...)
	if c.lib != nil {
		preview := role.NewHuman(c.lib, c.items)
		preview.Now = func() time.Time { return c.Now() }
		c.Inventory.Preview = preview
	}
	c.refreshEquipment()
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
		if before != c.InventoryState.Equipment {
			c.refreshEquipment()
		}
		return
	}
	switch p[1] {
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
