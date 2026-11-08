package app

import (
	"time"

	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const (
	pkButton         = 2
	pkTargetDistance = 270 // FUN_0038d3e8 requires each axis distance < 0x10f.
	pkNoticeDuration = 2 * time.Second
)

func (c *Client) initPK() {
	c.FuncButtons.Buttons[pkButton].OnClick = c.togglePK
	c.FuncButtons.Buttons[joinBattleButton].OnClick = func() { c.toggleBattleEntry(protocol.BattleJoinPlayer) }
	c.FuncButtons.Buttons[watchBattleButton].OnClick = func() { c.toggleBattleEntry(protocol.BattleWatchPlayer) }
}

func (c *Client) setPKTarget(active bool) {
	if active {
		c.setBattleEntryTarget(0)
	}
	c.pkTarget = active
	if c.FuncButtons != nil && c.FuncButtons.Buttons[pkButton] != nil {
		c.FuncButtons.Buttons[pkButton].IconDown = active
	}
	c.syncTargetSelection()
}

// FUN_003a9020 arms sword targeting; a follower cannot initiate a challenge.
func (c *Client) togglePK() {
	if c.pkTarget {
		c.setPKTarget(false)
		return
	}
	if !c.canTargetPK() {
		return
	}
	c.cancelFieldSelection()
	c.World.StopWalk()
	c.resetGroundHold()
	c.setPKTarget(true)
}

func (c *Client) canTargetPK() bool {
	if c.toolbarBlocked(pkButton) || !c.Inventory.CanAct() || c.battle.state.Active || c.UI.Modal != nil {
		return false
	}
	if c.inTeam() && c.TeamState.Leader != c.World.Player.ID {
		c.friendNotice(localization.UI("ui.pk.leader_only"), pkNoticeDuration)
		return false
	}
	if c.SettingsState != nil && c.SettingsState.Synced && !c.SettingsState.Permissions.PKAllowed {
		c.friendNotice(localization.UI("ui.pk.disabled"), pkNoticeDuration)
		return false
	}
	return true
}

// FUN_0038d3e8 submits AC11:2:3 with a player ID and a zero NPC click ID.
// Both native and Gonline servers remain authoritative about PK availability.
func (c *Client) pkTargetAt(x, y int) bool {
	if !c.pkTarget {
		return false
	}
	c.setPKTarget(false)
	if !c.canTargetPK() {
		return true
	}
	id := c.World.PKTargetAt(x, y)
	peer := c.World.Peers[id]
	if id == 0 || peer == nil {
		return true
	}
	for _, member := range c.TeamState.Members {
		if member == id {
			c.friendNotice(localization.UI("ui.pk.same_team"), pkNoticeDuration)
			return true
		}
	}
	dx, dy := peer.X-c.World.Player.X, peer.Y-c.World.Player.Y
	if dx < -pkTargetDistance || dx > pkTargetDistance || dy < -pkTargetDistance || dy > pkTargetDistance {
		c.friendNotice(localization.UI("ui.app.too_far_0ef2cb2c"), pkNoticeDuration)
		return true
	}
	if err := c.Net.Send(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateChallenge, protocol.BattleChallengePlayer}.U32(id).U16(0)); err != nil {
		c.friendNotice(err.Error(), pkNoticeDuration)
	}
	return true
}
