package app

import "wonderland-gonline/internal/protocol"

const (
	joinBattleButton    = 3
	watchBattleButton   = 4
	battleEntryDistance = 170
)

// Native spectator Exit is the same AC11:1:3 button branch as the fight's
// escape request, but it must never be converted into an AC50 Flee action.
func (c *Client) leaveSpectatedBattle() {
	if c.battle.state.Active && c.battle.state.Spectating {
		_ = c.Net.Send([]byte{protocol.CommandBattleState, protocol.BattleStateExit, protocol.BattleExitRequest})
	}
}

func (c *Client) setBattleEntryTarget(mode byte) {
	c.battleEntryTarget = mode
	if c.FuncButtons != nil {
		for tag, selector := range map[int]byte{joinBattleButton: protocol.BattleJoinPlayer, watchBattleButton: protocol.BattleWatchPlayer} {
			if b := c.FuncButtons.Buttons[tag]; b != nil {
				b.IconDown = mode == selector
			}
		}
	}
	c.syncTargetSelection()
}

func (c *Client) canEnterBattle() bool {
	return !c.toolbarBlocked(joinBattleButton) && c.Inventory.CanAct() && !c.battle.state.Active && c.UI.Modal == nil && (!c.inTeam() || c.TeamState.Leader == c.World.Player.ID)
}

func (c *Client) toggleBattleEntry(mode byte) {
	if c.battleEntryTarget == mode {
		c.setBattleEntryTarget(0)
		return
	}
	if !c.canEnterBattle() {
		return
	}
	c.cancelFieldSelection()
	c.World.StopWalk()
	c.resetGroundHold()
	c.setBattleEntryTarget(mode)
}

func (c *Client) battleEntryTargetAt(x, y int) bool {
	mode := c.battleEntryTarget
	if mode == 0 {
		return false
	}
	c.setBattleEntryTarget(0)
	if !c.canEnterBattle() {
		return true
	}
	id := c.World.BattleTargetAt(x, y)
	peer := c.World.Peers[id]
	if peer == nil {
		return true
	}
	dx, dy := peer.X-c.World.Player.X, peer.Y-c.World.Player.Y
	if dx < -battleEntryDistance || dx > battleEntryDistance || dy < -battleEntryDistance || dy > battleEntryDistance {
		return true
	}
	if members := c.World.TeamMembers(id); len(members) > 0 {
		id = members[0]
	}
	if err := c.Net.Send(protocol.Builder{protocol.CommandBattleState, protocol.BattleStateChallenge, mode}.U32(id).U16(0)); err != nil {
		c.friendNotice(err.Error(), pkNoticeDuration)
	}
	return true
}
