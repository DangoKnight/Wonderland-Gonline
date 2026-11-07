package app

import (
	"image"
	"time"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/mall"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func (c *Client) initMall() {
	f := mall.NewForm(c.Env, c.items, c.Stats)
	c.Mall = f
	f.Now = func() time.Time { return c.Now() }
	f.LoadPictures = c.loadPictures
	f.Bag = func() game.Inventory { return c.InventoryState.Bag }
	f.Confirm = func(prompt string, accept func()) { c.confirm(clientassets.Big5Text(prompt), accept) }
	f.ApplyForge = func(slot, progress byte) bool {
		if slot < 1 || int(slot) > len(c.InventoryState.Bag) {
			return false
		}
		c.InventoryState.Bag[slot-1].SetForge(progress)
		return true
	}
	vouchers := clientassets.DefaultMallVouchers()
	companion := func(item uint16) world.NPCTemplate {
		if c.npcTemplates == nil {
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
		}
		return c.npcTemplates[uint32(vouchers[item])]
	}
	f.RewardName = func(item uint16) string { return companion(item).Name }
	f.RewardIcon = func(item uint16, r image.Rectangle) bool {
		if c.resources == nil {
			return false
		}
		if portrait := c.resources.petPortrait(companion(item)); portrait != nil {
			// Native Article companion faces are smaller than the 32px item cell.
			c.Screen.DrawStretch(r.Inset(4), portrait, true)
			return true
		}
		return false
	}
	f.Equipment = func() game.Equipment { return c.InventoryState.Equipment }
	f.CanFit = func(rows []mall.Row) bool {
		bag := c.InventoryState.Bag
		definitions := map[uint16]game.ItemDefinition{}
		for id, n := range c.items {
			definitions[id] = n.Definition
		}
		for _, row := range rows {
			n, ok := c.items[row.Entry.ID]
			if !ok {
				return false
			}
			if _, err := bag.Grant(game.Item{ID: row.Entry.ID}, int(row.Entry.Count)*int(row.Quantity), n.Definition.StackLimit(), definitions); err != nil {
				return false
			}
		}
		return true
	}
	f.Send = c.Net.Send
	f.Notice = func(message string) { c.Notices.Show([]byte(message), serviceNoticeDuration, c.Now()) }
	f.PlayerName = func() []byte {
		if c.Inventory != nil {
			return c.Inventory.PlayerName
		}
		return nil
	}
	var fitting *role.Human
	var fittingKey [10]uint32
	f.Preview = func(x, y, direction int) {
		if f.Fitted != (game.Equipment{}) && c.World != nil && c.lib != nil {
			p := c.World.Player
			slot := &login.CharacterSlot{Body: p.Body, Head: p.Head, Color1: p.Color1, Color2: p.Color2}
			key := [10]uint32{uint32(p.Body), uint32(p.Head), p.Color1, p.Color2}
			for i, item := range c.InventoryState.Equipment {
				if f.Fitted[i].ID != 0 {
					item = f.Fitted[i]
				}
				slot.Equipment[i+1] = item.ID
				key[i+4] = uint32(item.ID)
			}
			if fitting == nil {
				fitting = role.NewHuman(c.lib, c.items)
				fitting.Now = func() time.Time { return c.Now() }
			}
			if key != fittingKey {
				fitting.SetCharacter(slot)
				fittingKey = key
			}
			fitting.DrawBody(c.Screen, x, y, int32(direction))
			return
		}
		if c.Inventory != nil && c.Inventory.Preview != nil {
			c.Inventory.Preview.DrawBody(c.Screen, x, y, int32(direction))
		}
	}
	c.UI.Add(f)
	c.FuncButtons.Mall.OnClick = c.toggleMall
}
func (c *Client) toggleMall() {
	if c.Mall.Visible {
		c.Mall.Hide()
		return
	}
	if !c.canOpenModule() || c.UI.Modal != nil || c.battle.state.Active || c.Trade != nil && c.Trade.Partner != 0 || c.event.active {
		return
	}
	c.cancelFieldSelection()
	c.Mall.Show()
	if c.Mall.State.Catalog[c.Mall.State.Index()] == nil && !c.Mall.State.Busy() && len(c.Mall.State.Cart) == 0 {
		// Request missing definitions only. Refreshing an already loaded catalog
		// would invalidate a cart retained while the window was closed.
		if err := c.Net.Send([]byte{protocol.CommandMall, protocol.MallRefresh}); err != nil {
			c.Mall.Notice(err.Error())
		}
	}
	if c.World != nil {
		c.World.StopWalk()
	}
}
func (c *Client) MallKey(key uint16) bool {
	if key == escapeKey && c.Mall != nil && c.Mall.ContentsForm.Visible {
		c.Mall.ContentsForm.Hide()
		return true
	}
	if key != escapeKey || c.Mall == nil || !c.Mall.Visible || c.UI.Modal != nil {
		return false
	}
	c.Mall.Hide()
	return true
}
func (c *Client) mallPacket(p []byte) bool {
	if c.Mall == nil || len(p) < 2 {
		return false
	}
	if p[0] == protocol.CommandMall {
		if p[1] == protocol.MallForgeResult {
			if !c.Mall.ForgeReply(p) && c.Unhandled != nil {
				c.Unhandled(p)
			}
			return true
		}
		switch p[1] {
		case protocol.MallPointsCatalog, protocol.MallBonusCatalog, protocol.MallPointsBalance, protocol.MallBonusBalance, protocol.MallCartReceipt, protocol.MallSettings, protocol.MallStatus:
			if !c.Mall.Receive(p) && c.Unhandled != nil {
				c.Unhandled(p)
			}
			return true
		}
	}
	if p[0] == protocol.CommandCharacterSelection && p[1] == protocol.CharacterSelectionMallBalance {
		if c.sport != nil {
			c.sportBalance(p[1:])
		}
		if !c.Mall.Balance(p) && c.Unhandled != nil {
			c.Unhandled(p)
		}
		return true
	}
	return false
}
