package server

import "time"

// idleReadDeadline uses the login limit until a character is selected or
// created. Map loading and warps retain the gameplay limit even while not ready.
// A zero timeout clears the previous deadline so idle gameplay stays connected.
func (s *Server) idleReadDeadline(c *Session, now time.Time) time.Time {
	timeout := s.Config.IdleTimeout()
	s.worldMu.Lock()
	if c.character != nil {
		timeout = s.Config.WorldIdleTimeout()
	}
	s.worldMu.Unlock()
	if timeout == 0 {
		return time.Time{}
	}
	return now.Add(timeout)
}
