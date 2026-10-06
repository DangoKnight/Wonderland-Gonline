package server

import (
	"errors"
	"strings"
	"wonderland-gonline/internal/protocol"
)

// AdminAnnouncement keeps UI color choices tied to native chat channels.
func (s *Server) AdminAnnouncement(text, channel string) error {
	codes := map[string]byte{"notice": protocol.ChatGMMessage, "world": protocol.ChatWorldMessage, "guild": protocol.ChatGuildMessage, "whisper": protocol.ChatWhisperMessage, "banner": protocol.ChatHeadBanner}
	if channel == "" {
		channel = "notice"
	}
	code, ok := codes[channel]
	if !ok {
		return errors.New("invalid announcement channel")
	}
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' || r == '|' })
	if len(lines) == 0 {
		return errors.New("empty announcement")
	}
	packets := make([][]byte, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !validChatText(line, chatCommandMaxBytes) {
			return errors.New("invalid announcement")
		}
		if code == protocol.ChatHeadBanner {
			packets = append(packets, headBanner(line))
		} else {
			packets = append(packets, protocol.Builder{protocol.CommandChat, code}.U32(0).Bytes([]byte(line)))
		}
	}
	if len(packets) == 0 {
		return errors.New("empty announcement")
	}
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.mu.Lock()
	recipients := make([]*Session, 0, len(s.sessions))
	for _, c := range s.sessions {
		if c.info.CharacterID != 0 {
			recipients = append(recipients, c)
		}
	}
	s.mu.Unlock()
	for _, c := range recipients {
		for _, p := range packets {
			if err := c.send(p); err != nil {
				c.conn.Close()
				break
			}
		}
	}
	return nil
}
