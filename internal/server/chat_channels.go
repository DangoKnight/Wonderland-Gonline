package server

import (
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

// chatPacket is 2/sub with the speaker's ID and the text, the layout the
// client reads for every chat channel (2/1 World, 2/2 Local, 2/3 Whisper,
// 2/5 Team).
func chatPacket(sub byte, sender *Session, text []byte) []byte {
	return protocol.Builder{protocol.CommandChat, sub}.U32(sender.character.ID).Bytes(text)
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

// worldChat is 2/1 to every player in the world. The client logs its own
// line when it sends 2/1, so only the chat commands echo it.
func (s *Server) worldChat(c *Session, text string, echo bool) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Chat messages must contain 1–60 bytes without control characters.")
	}
	recipients := make([]*Session, 0, len(s.world))
	for _, peer := range s.world {
		recipients = append(recipients, peer)
	}
	s.deliverChat(c, recipients, game.ChatChannelWorld, chatPacket(protocol.ChatWorldMessage, c, []byte(text)), echo)
	return nil
}

// teamChat is 2/5 to the party, the sender included: the client does not
// log its own Team line.
func (s *Server) teamChat(c *Session, text string) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Usage: /team <message> (up to 60 bytes)")
	}
	if c.party == nil {
		return s.chatFeedback(c, "You are not in a party.")
	}
	s.deliverChat(c, c.party.members, game.ChatChannelTeam, chatPacket(protocol.ChatTeamMessage, c, []byte(text)), true)
	return nil
}

// whisper is 2/3 from the sender to the target, echoed to the sender: the
// client does not log its own whisper.
func (s *Server) whisper(c *Session, target *Session, text string) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Usage: /whisper <character ID or name> <message>")
	}
	if target == nil || target.character.Preferences().Channels&game.ChatChannelWhisper == 0 {
		return s.chatFeedback(c, "That character is unavailable for whispers.")
	}
	recipients := []*Session{target}
	if target != c {
		recipients = append(recipients, c)
	}
	s.deliverChat(c, recipients, game.ChatChannelWhisper, chatPacket(protocol.ChatWhisperMessage, c, []byte(text)), true)
	return nil
}

// onlineByID is a ready player in the world by character ID.
func (s *Server) onlineByID(id uint32) *Session {
	for _, peer := range s.world {
		if peer.character != nil && peer.ready && peer.character.ID == id {
			return peer
		}
	}
	return nil
}

// Public channel commands are handled before the administrative command gate.
func (s *Server) chatChannelCommand(c *Session, text string) (bool, error) {
	header, body, _ := strings.Cut(text, " ")
	switch strings.ToLower(header) {
	case "/world", ":world", "/global", ":global":
		return true, s.worldChat(c, body, true)
	case "/team", ":team", "/party", ":party", "/p":
		return true, s.teamChat(c, body)
	case "/whisper", ":whisper", "/w", "/tell":
		targetName, message, _ := strings.Cut(strings.TrimSpace(body), " ")
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
		return true, s.whisper(c, target, message)
	}
	return false, nil
}
