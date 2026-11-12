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
	case protocol.HandshakeAgentAccessDenied:
		if protocol.IsAgentAccessDenied(p) {
			c.agent.serverPermission.Store(agentPermissionDenied)
			c.agent.serverAttached.Store(true)
			c.agentReset()
			c.agentClearInput()
			c.logf("%s", protocol.AgentAccessDeniedMessage)
			c.disconnectText = []byte(protocol.AgentAccessDeniedMessage)
			c.Net.Close()
		}
		return true
	case protocol.HandshakeConnectionOffer:
		flags, ok := protocol.ParseConnectionCapabilities(p, p[1])
		if !ok || c.connection.offered || c.connection.enabled || c.combat.enabled || c.backdrops.enabled || c.battle.state.Active {
			return true
		}
		supported := uint32(protocol.ConnectionCollectionTiming | protocol.ConnectionCombatRoundChain | protocol.ConnectionCompleteMapSnapshot | protocol.ConnectionObservedPickups | protocol.ConnectionAFK | protocol.ConnectionAFKAggregation | protocol.ConnectionAFKDelta)
		if flags&protocol.ConnectionAFK == 0 {
			supported &^= protocol.ConnectionAFKAggregation | protocol.ConnectionAFKDelta
		}
		if c.resources != nil && len(c.resources.battleBackdrops) > 0 {
			supported |= protocol.ConnectionClientBackdrops
		}
		chosen := flags & supported
		if c.agentHosted() || c.replica != nil {
			// Declaration is mandatory even when the server did not offer access;
			// a denying server can then explain its policy and refuse the session.
			chosen |= protocol.ConnectionAgentAPI
			c.agent.serverAttached.Store(true)
			c.agent.serverPermission.Store(agentPermissionUnknown)
			if flags&protocol.ConnectionAgentAPI == 0 {
				c.agent.serverPermission.Store(agentPermissionDenied)
			}
		}
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
		if c.agentHosted() && flags&protocol.ConnectionAgentAPI != 0 {
			c.agent.serverPermission.Store(agentPermissionGranted)
		}
		c.gonlineCollectionTiming = flags&protocol.ConnectionCollectionTiming != 0
		c.combat.enabled = flags&protocol.ConnectionCombatRoundChain != 0
		c.backdrops.enabled = flags&protocol.ConnectionClientBackdrops != 0
		return true
	}
	return false
}
