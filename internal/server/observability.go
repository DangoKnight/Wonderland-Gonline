package server

import (
	serverlog "wonderland-gonline/internal/server/logging"
)

// LogEntry is the administration API log record.
type LogEntry = serverlog.LogEntry

func (s *Server) Logs() []LogEntry { return s.logs.Snapshot() }

// Read world state under its lock because another session can move this player.
func (s *Server) sessionLogAttrs(c *Session) []any {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	attrs := []any{"session", c.info.ID, "service", c.info.Service, "account", c.account.ID, "map_ready", c.ready}
	if c.character != nil {
		attrs = append(attrs, "character", c.character.ID, "map", c.character.Map)
	}
	return attrs
}
