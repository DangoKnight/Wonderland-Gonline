package server

import "wonderland-gonline/internal/store"

// returnToAccount handles bare AC63:0 from character selection. The client
// opens its account form locally and expects no reply or connection closure.
func (s *Server) returnToAccount(c *Session) error {
	// Match authentication's lock order so admin credential changes cannot race
	// release and subsequent authentication on this connection.
	s.accountMu.RLock()
	defer s.accountMu.RUnlock()
	s.releaseName(c)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accounts[c.account.ID] == c.info.ID {
		delete(s.accounts, c.account.ID)
	}
	c.account = store.Account{}
	c.gmLevel.Store(0)
	c.slot = 0
	c.info.Username = ""
	c.info.CharacterID, c.info.CharacterName, c.info.Map = 0, "", 0
	return nil
}
