package server

import "wonderland-gonline/internal/protocol"

// AC5:5 resets the actor shape. The reference server's proposed AC23:122 success effect is
// fishing cleanup in the verified WLRI client and must not be sent here.
func (s *Server) clearShapeshift(c *Session) {
	p := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateShapeshift}.U32(c.character.ID).U16(0)
	s.broadcastWorld(c, p)
	s.sendOrClose(c, p)
}
