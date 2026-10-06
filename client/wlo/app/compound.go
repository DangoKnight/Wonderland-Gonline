package app

import (
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/internal/game"
)

const mainCompoundButton = 3

func (c *Client) initCompound() {
	c.loadPictures("Compounding", "Btn_CompSk_1", "Btn_CompSk_2", "Btn_CompSk_3", "Icon_RoleSelectFrame", "Icon_UseCompoundSkill")
	c.Compound = inventory.NewCompoundForm(c.Env, c.Inventory)
	c.Compound.CanAct = c.Inventory.CanAct
	c.Compound.CanSelect = func(slot byte) bool {
		return c.InventoryState.Items[c.InventoryState.Bag[slot-1].ID].Definition.Type != game.ItemTypeExotic && c.World != nil && (c.World.Player.VehicleID == 0 || c.World.Player.VehicleSlot != slot)
	}
	c.Compound.SkillLearned = func(id uint16) bool { return c.SkillState.Learned[id].Grade > 0 }
	c.Compound.Now = func() time.Time { return c.Now() }
	c.Compound.Send = c.Net.Send
	c.Compound.Notice = c.Chat.Notice
	c.UI.Add(c.Compound)
	c.MainButtons.Buttons[mainCompoundButton].OnClick = c.toggleCompound
}
func (c *Client) toggleCompound() {
	if c.Compound.Visible {
		c.Compound.Hide()
	} else if c.Inventory.CanAct() && c.UI.Modal == nil {
		c.Compound.Show()
	}
}
func (c *Client) CompoundKey(key uint16, shift byte) bool {
	if key == 'C' && shift&shiftCtrl != 0 {
		c.toggleCompound()
		return true
	}
	if key == escapeKey && c.Compound.Visible && (c.UI.Active == c.Compound || c.Input.Focused != nil && c.Input.Focused.Base().Root() == c.Compound) {
		c.Compound.Hide()
		return true
	}
	return false
}
