package app

import (
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/protocol"
)

const nativeSitAction = 16
const nativeFacingSplit = 4
const nativeFacingCount = 8

// EmoteKey implements the default Alt+1 held sit pose. Native pose selection
// uses FUN_0027f248 -> the table at 0x4bd7bc -> FUN_00430f90. Sit has two
// directions (16/17); water riding is a separate seated group (46..53).
func (c *Client) EmoteKey(key uint16, shift byte) bool {
	if key != '1' || shift != shiftAlt || c.World == nil {
		return false
	}
	if _, typing := c.Input.Focused.(*seui.Editor); typing {
		return false
	}
	if c.held || c.event.active || c.sceneFrozen() || c.World.Player.VehicleID != 0 || c.remote.battle.active {
		return true
	}
	facing := int(c.World.Player.Direction) % nativeFacingCount
	pose := byte(nativeSitAction)
	if facing >= nativeFacingSplit {
		pose++
	}
	c.World.StopWalk()
	c.waterTravel = waterTravelState{}
	c.World.Player.Direction = int32(pose)
	c.Net.Send([]byte{protocol.CommandPose, protocol.PoseBroadcast, pose})
	return true
}
