package app

import (
	"encoding/binary"
	"time"

	"wonderland-go/internal/protocol"
)

// Chat packets after the command byte: subcommand, a character ID, then
// the text.
const (
	chatNotice      = 3 // 2/3, notice-board text like 2/16; not yet named in internal/protocol
	chatIDEnd       = 5
	chatNoticeFor   = 2000 * time.Millisecond
	chatMaxReceived = 0x3c // 2/2 cuts other players' lines to 60 characters
)

// welcome is FUN_00492ac4's first-entry line: the chat log empties
// (FUN_0048cab4), then "Welcome to [<server>] Server" is added as a
// system line, which also runs through the ticker.
func (c *Client) welcome() {
	c.Chat.Clear()
	line := append([]byte("Welcome to ["), c.G.ServerText...)
	c.Chat.Add(append(line, "] Server"...), 0)
}

// mapChat is 2/2 (0x2df343): a player's Local line, named from the map's
// players. The speech bubble over the speaker (FUN_00428ed0) is not
// ported.
func (c *Client) mapChat(s []byte) {
	if len(s) < chatIDEnd || c.World == nil {
		return
	}
	id := binary.LittleEndian.Uint32(s[1:])
	text := s[chatIDEnd:]
	name := c.World.PeerName(id)
	if len(text) > chatMaxReceived {
		text = text[:chatMaxReceived]
	}
	c.Chat.AddLocal(name, text)
}

// sendChat is FUN_002641c4 for the Local channel: 2/2 with the text, and
// the line is added to the log at once (the server does not echo it).
func (c *Client) sendChat() {
	m := c.ChatBar.Message
	if len(m.Text) == 0 || c.World == nil {
		return
	}
	text := append([]byte(nil), m.Text...)
	c.Net.Send(append([]byte{protocol.CommandChat, protocol.ChatMapMessage}, text...))
	c.Chat.AddLocal(c.World.Player.Name, text)
	m.Clear()
}
