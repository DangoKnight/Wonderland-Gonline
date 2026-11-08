package server

import "wonderland-gonline/internal/protocol"

func (c *Session) knowsPlayer(id uint32) bool {
	c.knownMu.Lock()
	defer c.knownMu.Unlock()
	return c.knownPlayers[id]
}

// Caller holds worldMu. Only previously disclosed characters are announced;
// unknown players and private tutorial populations remain undiscoverable.
func (s *Server) knownPlayerOnline(c *Session) {
	if c.invisible {
		return
	}
	appearance, err := c.character.AppearancePacket(true)
	if err != nil {
		return
	}
	for _, peer := range s.friendSessions {
		if peer != c && !s.samePlayerScene(c, peer) && peer.knowsPlayer(c.character.ID) {
			if peer.character.Map == c.character.Map {
				// Private tutorial/tent instances can share a map number. An AC4
				// here would spawn a peer that is deliberately absent from view.
				s.sendOrClose(peer, protocol.Builder{protocol.CommandPresence, protocol.PresenceOnline}.U32(c.character.ID).U8(protocol.PresenceMapAvailable))
			} else {
				s.sendOrClose(peer, appearance)
			}
		}
	}
}

func (s *Server) knownPlayerOffline(c *Session) {
	packet := protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.character.ID).U16(0).U16(0).U16(0).U16(0).U8(0)
	for _, peer := range s.friendSessions {
		if peer != c && !s.samePlayerScene(c, peer) && peer.knowsPlayer(c.character.ID) {
			s.sendOrClose(peer, packet)
		}
	}
}
