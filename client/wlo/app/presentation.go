package app

import (
	"encoding/binary"
	"wonderland-go/internal/protocol"
)

const poseReplyBytes = 6 // subcommand, character ID, expression/action

// receivePose follows the AC32 receive branch at 0x2ea560 and the role
// helpers FUN_00432478 and FUN_00438684.
func (c *Client) receivePose(p []byte) {
	if c.World == nil || len(p) == 0 {
		return
	}
	switch p[0] {
	case protocol.PoseEmote:
		if len(p) != poseReplyBytes {
			return
		}
		c.World.StartExpression(binary.LittleEndian.Uint32(p[1:]), p[5], c.Now())
	case protocol.PoseBroadcast:
		c.World.ApplyPose(p)
	}
}

const (
	presenceIDEnd        = 5
	presenceNameUpdate   = 5 // inbound AC10 name record at 0x2e1caa
	presenceStatusUpdate = 2 // native +0x20af update at 0x2e1b06
	presenceStringEnd    = presenceIDEnd + 1
)

// receivePresence ports AC10 name/nickname and presence updates at
// 0x2e1996..0x2e1e58. Presence is presentation metadata, not a map despawn.
func (c *Client) receivePresence(p []byte) {
	if c.World == nil || len(p) < presenceStringEnd {
		return
	}
	id := binary.LittleEndian.Uint32(p[1:])
	if id == 0 {
		return
	}
	var value []byte
	switch p[0] {
	case protocol.SocialNicknameUpdate, presenceNameUpdate:
		n := int(p[presenceIDEnd])
		if len(p) != presenceStringEnd+n {
			return
		}
		value = append([]byte(nil), p[presenceStringEnd:]...)
		if p[0] == presenceNameUpdate {
			c.rememberPlayer(id, value)
			if id == c.World.Player.ID {
				c.World.Player.Name = value
				if c.Inventory != nil {
					c.Inventory.PlayerName = append([]byte(nil), value...)
				}
			}
			if peer := c.World.Peers[id]; peer != nil {
				peer.Name = value
			}
		} else {
			if id == c.World.Player.ID {
				c.World.Player.Nickname = value
			}
			if peer := c.World.Peers[id]; peer != nil {
				peer.Nickname = value
			}
		}
	case presenceStatusUpdate, protocol.PresenceOnline:
		if len(p) != presenceStringEnd {
			return
		}
		if id == c.World.Player.ID {
			c.World.Player.Presence = p[presenceIDEnd]
		}
		if peer := c.World.Peers[id]; peer != nil {
			peer.Presence = p[presenceIDEnd]
		}
	}
}
