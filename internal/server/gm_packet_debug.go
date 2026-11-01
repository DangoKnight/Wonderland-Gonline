package server

import (
	"context"
	"strings"
)

// Reuse the existing durable log-level control. Packet traces retain their
// command-specific bounds and credential redaction when debugging is enabled.
func (s *Server) gmPacketDebug(ctx context.Context, c *Session, _ string, args []string) error {
	if len(args) > 1 {
		return s.chatFeedback(c, "/packetdebug [on|off]")
	}
	enabled := s.logs.Level() != "debug"
	if len(args) == 1 {
		switch strings.ToLower(args[0]) {
		case "on", "true", "1", "enable":
			enabled = true
		case "off", "false", "0", "disable":
			enabled = false
		default:
			return s.chatFeedback(c, "/packetdebug [on|off]")
		}
	}
	level := "info"
	if enabled {
		level = "debug"
	}
	if err := s.Store.SaveSettings(ctx, map[string]string{"log_level": level}); err != nil {
		return err
	}
	s.logs.SetLevel(level)
	return s.chatFeedback(c, "Packet diagnostics: "+level)
}
