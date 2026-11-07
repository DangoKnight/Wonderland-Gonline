package app

import "wonderland-gonline/client/wlo/worldpk"

const worldPKButton = 1

func (c *Client) initWorldPK() {
	c.WorldPK = worldpk.NewForm(c.Env)
	c.UI.Add(c.WorldPK)
	c.FuncButtons.Buttons[worldPKButton].OnClick = c.toggleWorldPK
}

func (c *Client) toggleWorldPK() {
	if c.WorldPK.Visible {
		c.WorldPK.Hide()
		return
	}
	if !c.canOpenModule() || c.battle.state.Active || c.UI.Modal != nil || c.toolbarBlocked(worldPKButton) {
		return
	}
	c.cancelFieldSelection()
	c.WorldPK.Show()
}

func (c *Client) WorldPKKey(key uint16) bool {
	if key != escapeKey || c.WorldPK == nil || !c.WorldPK.Visible || c.UI.Modal != nil {
		return false
	}
	c.WorldPK.Hide()
	return true
}
