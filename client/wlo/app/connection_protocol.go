package app

import "wonderland-gonline/internal/protocol"

type connectionRuntime struct {
	offered, enabled bool
	features         uint32
}

func (c *Client) connectionCapabilitiesPacket(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandHandshake {
		return false
	}
	switch p[1] {
	case protocol.HandshakeConnectionOffer:
		flags, ok := protocol.ParseConnectionCapabilities(p, p[1])
		if !ok || c.connection.offered || c.connection.enabled || c.combat.enabled || c.backdrops.enabled || c.battle.state.Active {
			return true
		}
		supported := uint32(protocol.ConnectionCollectionTiming | protocol.ConnectionCombatRoundChain | protocol.ConnectionCompleteMapSnapshot)
		if c.resources != nil && len(c.resources.battleBackdrops) > 0 {
			supported |= protocol.ConnectionClientBackdrops
		}
		chosen := flags & supported
		if c.Net.Send(protocol.ConnectionCapabilities(protocol.HandshakeConnectionAccept, chosen)) == nil {
			c.connection = connectionRuntime{offered: true, features: chosen}
		}
		return true
	case protocol.HandshakeConnectionEnabled:
		flags, ok := protocol.ParseConnectionCapabilities(p, p[1])
		if !ok || !c.connection.offered || c.connection.enabled || flags != c.connection.features {
			return true
		}
		c.connection.enabled = true
		c.gonlineCollectionTiming = flags&protocol.ConnectionCollectionTiming != 0
		c.combat.enabled = flags&protocol.ConnectionCombatRoundChain != 0
		c.backdrops.enabled = flags&protocol.ConnectionClientBackdrops != 0
		return true
	}
	return false
}
