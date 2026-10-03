package server

import (
	"fmt"
	"strconv"
	"strings"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

const (
	chatMessageMaxBytes = 60
	chatCommandMaxBytes = 512
)

func validChatText(text string, limit int) bool {
	if len(text) == 0 || len(text) > limit || strings.TrimSpace(text) == "" {
		return false
	}
	for _, b := range []byte(text) {
		if b < 32 || b == 127 {
			return false
		}
	}
	return true
}

// Extra channel commands use the verified native head-banner packet until the
// original client's dedicated whisper/team/world receive layouts are ported.
func chatChannelPacket(label string, sender *Session, text string) []byte {
	return protocol.Builder{protocol.CommandChat, protocol.ChatHeadBanner}.U32(0).Bytes([]byte(fmt.Sprintf("(%s) %s: %s", label, sender.character.Name, text)))
}

func (s *Server) deliverChat(sender *Session, recipients []*Session, channel byte, packet []byte, echo bool) {
	for _, peer := range recipients {
		if peer == nil || !peer.ready || peer.character == nil || (peer == sender && !echo) || peer.character.Preferences().Channels&channel == 0 {
			continue
		}
		if err := peer.send(packet); err != nil {
			peer.conn.Close()
		}
	}
}

func (s *Server) chatFeedback(c *Session, text string) error {
	return c.send(headBanner(text))
}

func (s *Server) worldChat(c *Session, text string) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Chat messages must contain 1–60 bytes without control characters.")
	}
	recipients := make([]*Session, 0, len(s.world))
	for _, peer := range s.world {
		recipients = append(recipients, peer)
	}
	s.deliverChat(c, recipients, game.ChatChannelWorld, chatChannelPacket("World", c, text), true)
	return nil
}

// Public channel commands are handled before the administrative command gate.
func (s *Server) chatChannelCommand(c *Session, text string) (bool, error) {
	header, body, _ := strings.Cut(text, " ")
	switch strings.ToLower(header) {
	case "/world", ":world", "/global", ":global":
		return true, s.worldChat(c, body)
	case "/team", ":team", "/party", ":party", "/p":
		if !validChatText(body, chatMessageMaxBytes) {
			return true, s.chatFeedback(c, "Usage: /team <message> (up to 60 bytes)")
		}
		if c.party == nil {
			return true, s.chatFeedback(c, "You are not in a party.")
		}
		s.deliverChat(c, c.party.members, game.ChatChannelTeam, chatChannelPacket("Team", c, body), true)
		return true, nil
	case "/whisper", ":whisper", "/w", "/tell":
		targetName, message, _ := strings.Cut(strings.TrimSpace(body), " ")
		if !validChatText(message, chatMessageMaxBytes) {
			return true, s.chatFeedback(c, "Usage: /whisper <character ID or name> <message>")
		}
		var target *Session
		id, err := strconv.ParseUint(targetName, 10, 32)
		for _, peer := range s.world {
			matches := peer.character != nil && strings.EqualFold(peer.character.Name, targetName)
			if err == nil {
				matches = peer.character != nil && peer.character.ID == uint32(id)
			}
			if peer.character != nil && peer.ready && matches {
				target = peer
				break
			}
		}
		if target == nil || target.character.Preferences().Channels&game.ChatChannelWhisper == 0 {
			return true, s.chatFeedback(c, "That character is unavailable for whispers.")
		}
		s.deliverChat(c, []*Session{target}, game.ChatChannelWhisper, chatChannelPacket("Whisper", c, message), true)
		if target != c {
			return true, s.chatFeedback(c, fmt.Sprintf("(Whisper to %s) %s", target.character.Name, message))
		}
		return true, nil
	}
	return false, nil
}
