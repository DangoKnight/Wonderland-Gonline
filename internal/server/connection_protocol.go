package server

import "wonderland-gonline/internal/protocol"

// Caller holds worldMu. Offers are captured once; repeated discovery does not
// renegotiate features or inspect mutable public startup configuration.
func (s *Server) offerConnectionProtocol(c *Session) error {
	if c.connectionOffered != 0 || c.connectionNegotiated {
		return nil
	}
	flags := uint32(protocol.ConnectionCollectionTiming | protocol.ConnectionClientBackdrops | protocol.ConnectionCompleteMapSnapshot | protocol.ConnectionObservedPickups)
	if s.combatRoundChain {
		flags |= protocol.ConnectionCombatRoundChain
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
