package app

// cancelFieldSelection cancels only an unsubmitted targeting choice. It never
// withdraws a sent action, changes AUTO, or closes unrelated modules.
func (c *Client) cancelFieldSelection() bool {
	active := c.pkTarget || c.joinTeamTarget || c.friendTarget || c.tradeTarget || c.battle.binding != nil || c.battle.item != nil || c.hotbar.target != nil
	if !active {
		return false
	}
	c.setJoinTeamTarget(false)
	c.setFriendTarget(false)
	c.setTradeTarget(false)
	c.setPKTarget(false)
	c.battle.binding, c.battle.item = nil, nil
	c.closeHotbarTarget()
	c.resetGroundHold()
	if c.battle.state.Active {
		c.battleMenu()
	}
	c.updateCursor(c.Now())
	return true
}
