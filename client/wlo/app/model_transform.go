package app

import (
	"encoding/binary"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/world"
)

const (
	modelTransformPacketBytes = 7 // Command, subcommand, character UInt32, model UInt8.
	modelTransformIDOffset    = 2
	modelTransformModelOffset = 6
)

// receiveModelTransform mirrors FUN_00451110: replace body/head in place,
// including cached teammates. The following equipment packet supplies clothing.
func (c *Client) receiveModelTransform(p []byte) {
	if len(p) != modelTransformPacketBytes || c.World == nil {
		return
	}
	body, head, ok := login.ModelAppearance(p[modelTransformModelOffset])
	if !ok {
		return
	}
	id := binary.LittleEndian.Uint32(p[modelTransformIDOffset:])
	if id == c.World.Player.ID {
		c.World.Player.Body, c.World.Player.Head = body, head
		c.refreshEquipment()
	}
	if peer := c.World.Peers[id]; peer != nil {
		peer.Body, peer.Head = body, head
		world.Dress(peer.Role, peer.Player)
	}
	if peer := c.teamAppearances[id]; peer != nil && peer != c.World.Peers[id] {
		peer.Body, peer.Head = body, head
		world.Dress(peer.Role, peer.Player)
	}
}
