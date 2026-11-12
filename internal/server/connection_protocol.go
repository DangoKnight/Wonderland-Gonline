package server

import "wonderland-gonline/internal/protocol"

// Caller holds worldMu. Offers are captured once; repeated discovery does not
// renegotiate features or inspect mutable public startup configuration.
func (s *Server) offerConnectionProtocol(c *Session) error {
	if c.connectionOffered != 0 || c.connectionNegotiated {
		return nil
	}
	flags := uint32(protocol.ConnectionCollectionTiming | protocol.ConnectionClientBackdrops | protocol.ConnectionCompleteMapSnapshot | protocol.ConnectionObservedPickups | protocol.ConnectionAFK | protocol.ConnectionAFKAggregation | protocol.ConnectionAFKDelta)
	if s.combatRoundChain {
		flags |= protocol.ConnectionCombatRoundChain
	}
	if s.agentAPIEnabled {
		flags |= protocol.ConnectionAgentAPI
	}
	if err := c.send(protocol.ConnectionCapabilities(protocol.HandshakeConnectionOffer, flags)); err != nil {
		return err
	}
	c.connectionOffered = flags
	return nil
}

func (s *Server) acceptConnectionProtocol(c *Session, p []byte) error {
	flags, ok := protocol.ParseConnectionCapabilities(p, protocol.HandshakeConnectionAccept)
	if !ok {
		return protocol.ErrMalformed
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if flags&protocol.ConnectionAgentAPI != 0 {
		c.agentRequested = true
		if !s.agentAPIEnabled || c.connectionOffered&protocol.ConnectionAgentAPI == 0 {
			return s.denyAgentAccess(c)
		}
	}
	if c.agentRequested && flags&protocol.ConnectionAgentAPI == 0 {
		if !s.agentAPIEnabled {
			return s.denyAgentAccess(c)
		}
		return protocol.ErrMalformed
	}
	if flags&(protocol.ConnectionAFKAggregation|protocol.ConnectionAFKDelta) != 0 && flags&protocol.ConnectionAFK == 0 {
		return protocol.ErrMalformed
	}
	if c.connectionOffered == 0 || flags & ^c.connectionOffered != 0 {
		return protocol.ErrMalformed
	}
	if c.connectionNegotiated {
		if flags == c.connectionFeatures {
			return nil
		} // Idempotent, never switches an established connection.
		return protocol.ErrMalformed
	}
	if c.battle != nil {
		return protocol.ErrMalformed
	}
	if err := c.send(protocol.ConnectionCapabilities(protocol.HandshakeConnectionEnabled, flags)); err != nil {
		return err
	}
	c.connectionFeatures, c.connectionNegotiated = flags, true
	c.clientBackdrops = flags&protocol.ConnectionClientBackdrops != 0
	c.combatCapabilities = 0
	if flags&protocol.ConnectionCombatRoundChain != 0 {
		c.combatCapabilities = protocol.CombatCapabilityRoundChain
	}
	return nil
}

// Caller holds worldMu. Refusal is queued before closing the game connection.
func (s *Server) denyAgentAccess(c *Session) error {
	s.Log.Info("agent/API access refused by server policy", "session", c.info.ID, "account", c.account.ID)
	err := c.send(protocol.AgentAccessDenied())
	c.closeAfterFlush()
	return err
}

func (s *Server) revokeAgentAccess() {
	s.mu.Lock()
	var agents []*Session
	for _, c := range s.sessions {
		if c.agentRequested {
			agents = append(agents, c)
		}
	}
	s.mu.Unlock()
	for _, c := range agents {
		_ = s.denyAgentAccess(c)
	}
}
