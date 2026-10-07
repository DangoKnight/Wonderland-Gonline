package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/internal/protocol"
)

// Chat packets after the command byte: subcommand, a character ID, then
// the text.
const (
	chatIDEnd       = 5
	chatNoticeFor   = 2000 * time.Millisecond
	chatMaxReceived = 0x3c // other players' lines are cut to 60 characters
)

// welcome is FUN_00492ac4's first-entry line: the chat log empties
// (FUN_0048cab4), the bar resets (FUN_00268980), then "Welcome to
// [<server>] Server" is added on channel 0 with the player as speaker, so
// it has no prefix and also runs through the ticker.
func (c *Client) welcome() {
	c.Chat.Clear()
	c.ChatBar.Reset()
	line := append([]byte("Welcome to ["), c.G.ServerText...)
	c.Chat.Say(c.World.Player.ID, nil, append(line, "] Server"...), hud.ChannelSystem)
}

// receiveChat is 2/0..2/7 (the receive cases at 0x2df0fb…0x2dfa42): the
// speaker's ID and the text, shown on channel n with the speaker's name.
// A Team line needs a known speaker. The speech bubble over the speaker
// (FUN_00428ed0) is not ported.
func (c *Client) receiveChat(sub byte, s []byte) {
	if len(s) < chatIDEnd || c.World == nil {
		return
	}
	id := binary.LittleEndian.Uint32(s[1:])
	text := s[chatIDEnd:]
	if sub != protocol.ChatSystemMessage && sub != protocol.ChatGMMessage && len(text) > chatMaxReceived {
		text = text[:chatMaxReceived]
	}
	name := c.playerName(id)
	if c.SettingsState != nil && id != c.World.Player.ID && sub != protocol.ChatSystemMessage && sub != protocol.ChatGMMessage && c.SettingsState.Local.Blocked(name) {
		return
	}
	if sub == protocol.ChatTeamMessage && name == nil {
		return
	}
	c.Chat.Say(id, name, text, int(sub))
	// Another player's whisper joins the recent whisperers (FUN_0048ac28
	// case 3 → FUN_00268cdc).
	if sub == protocol.ChatWhisperMessage && id != c.World.Player.ID && len(name) > 0 {
		c.ChatBar.RememberWhisperer(name, false)
	}
}

// chatFace is a speaker's look for the chat log's icon: the player, or a
// player on the map.
func (c *Client) chatFace(id uint32) hud.SpeakerFace {
	if c.World == nil {
		return nil
	}
	var body any
	if id == c.World.Player.ID {
		body = c.World.Body
	} else if p := c.World.Peers[id]; p != nil {
		body = p.Role
	}
	if f, ok := body.(hud.SpeakerFace); ok {
		return f
	}
	return nil
}

// Send rules of FUN_002641c4.
const (
	// World needs a radio in the special slot (equipment slot 6,
	// FUN_00452fc4 → FUN_003d1018) and 15 SP.
	radioSlot      = 6
	radioMinSP     = 0xf
	noRadio        = "(System):World Channel requires Radio Set"
	radioLowSP     = "(Alert):Not enough SP to use Radio Setq"
	noTarget       = "No target"
	targetOffline  = "Player is offline"
	noSuchPerson   = "<<No such person online>>"
	notInTeamChat  = "(System):Not in Team Chat anymore"
	notInGuildChat = "(System):Not in Guild Chat anymore"
	// Whisper is unavailable on map 0x29cd (a notice for 1.2 s).
	noWhisperMap       = 0x29cd
	noWhisperNotice    = "Temporary unavailable"
	noWhisperNoticeFor = 0x4b0 * time.Millisecond
	// "/msg <name> <text>" whispers (FUN_002687f0); the log notes it as
	// a channel-4 line.
	msgCommand = "/msg"
	msgLine    = "Whisper "
	// The whisper target line of FUN_00269068.
	whisperToOpen, whisperToClose = "Whisp to <<", ">>"
)

// radioItems are FUN_003d1018's radios: Radio Set, Loudspeaker and
// Transceiver.
var radioItems = map[uint16]bool{34076: true, 34132: true, 39070: true}

// sendChat is FUN_002641c4: the message goes out on the bar's channel,
// unless that channel's rule refuses it with a line in the log (the text
// stays). World and Local lines are added at once; the others come back
// from the server.
func (c *Client) sendChat() {
	bar := c.ChatBar
	m := bar.Message
	if len(m.Text) == 0 || c.World == nil {
		return
	}
	text := append([]byte(nil), m.Text...)
	if pending, ok := c.msgCommand(text); ok {
		text = pending
	}
	self := c.World.Player
	sub := byte(0)
	switch bar.Channel {
	case hud.InputWorld:
		if !c.hasRadio() {
			c.Chat.Notice(noRadio)
			return
		}
		if c.Stats != nil && c.Stats.SP < radioMinSP {
			c.Chat.Notice(radioLowSP)
			return
		}
		sub = protocol.ChatWorldMessage
		c.Chat.Say(self.ID, self.Name, text, hud.ChannelWorld)
	case hud.InputLocal:
		sub = protocol.ChatMapMessage
		c.Chat.Say(self.ID, self.Name, text, hud.ChannelLocal)
	case hud.InputWhisper:
		if self.Map == noWhisperMap {
			c.Notices.Show([]byte(noWhisperNotice), noWhisperNoticeFor, c.Now())
			return
		}
		if len(bar.Whisper.Text) == 0 {
			c.Chat.Notice(noTarget)
			bar.Target = 0
			return
		}
		if bar.Target == 0 {
			c.Chat.Notice(noTarget)
			bar.Remember()
			return
		}
		if c.playerName(bar.Target) == nil {
			c.Chat.Notice(targetOffline)
			bar.Target = 0
			return
		}
		p := binary.LittleEndian.AppendUint32([]byte{protocol.CommandChat, protocol.ChatWhisperMessage}, bar.Target)
		c.Net.Send(append(p, text...))
		bar.Remember()
		return
	case hud.InputTeam:
		if !c.inTeam() {
			c.Chat.Notice(notInTeamChat)
			bar.SelectChannel(hud.InputLocal)
			return
		}
		sub = protocol.ChatTeamMessage
	case hud.InputGuild:
		if !c.inGuild() {
			c.Chat.Notice(notInGuildChat)
			bar.SelectChannel(hud.InputLocal)
			return
		}
		sub = protocol.ChatGuildMessage
	default:
		return
	}
	c.Net.Send(append([]byte{protocol.CommandChat, sub}, text...))
	bar.Remember()
}

// msgCommand is FUN_002687f0: "/msg <name> <text>" switches to Whisper
// and, when the name is a player on the map, aims at them and leaves the
// text to send.
func (c *Client) msgCommand(text []byte) ([]byte, bool) {
	cmd, rest, _ := bytes.Cut(text, []byte(" "))
	if !bytes.Equal(cmd, []byte(msgCommand)) {
		return nil, false
	}
	name, body, _ := bytes.Cut(rest, []byte(" "))
	c.ChatBar.SelectChannel(hud.InputWhisper)
	id, ok := c.playerByName(name)
	if !ok {
		c.ChatBar.Target = 0
		return nil, false
	}
	c.ChatBar.Target = id
	c.Chat.Say(0, nil, append([]byte(msgLine), name...), hud.ChannelGM)
	return body, true
}

// whisperEnter is the whisper field's Enter (FUN_00268a30): the name is
// looked up among the players and becomes the target (FUN_00269068).
func (c *Client) whisperEnter() {
	bar := c.ChatBar
	if c.World == nil {
		return
	}
	if len(bar.Whisper.Text) == 0 {
		c.Chat.Notice(noTarget)
		bar.Target = 0
		return
	}
	id, ok := c.playerByName(bar.Whisper.Text)
	if !ok {
		c.Chat.Notice(noSuchPerson)
		bar.Target = 0
		bar.Whisper.Clear()
		return
	}
	c.whisperTo(id)
}

// SpeakerPress is FUN_00496ca4's chat check: a press while the pointer is
// over a speaker's icon in the chat log (and the channel list is closed)
// whispers to that speaker, without the "Whisp to" line (+0x1b4), and
// takes the press.
func (c *Client) SpeakerPress() bool {
	if c.World == nil || c.Chat == nil || !c.Chat.Visible || c.ChatBar.Frame.Visible || c.UI.Modal != nil || c.Chat.Hovered == 0 {
		return false
	}
	c.quietWhisper = true
	c.whisperTo(c.Chat.Hovered)
	c.quietWhisper = false
	return true
}

// whisperTo is FUN_00269068: the target is set (not the player), the log
// says "Whisp to <<name>>" (unless a portrait was pressed), and the bar
// moves to Whisper with the name in its field and the keyboard on the
// message.
func (c *Client) whisperTo(id uint32) {
	bar := c.ChatBar
	if id == bar.Target {
		return
	}
	bar.Target = id
	if id == c.World.Player.ID {
		bar.Target = 0
		return
	}
	name := c.playerName(id)
	if !c.quietWhisper {
		line := bytes.Join([][]byte{[]byte(whisperToOpen), name, []byte(whisperToClose)}, nil)
		c.Chat.Say(id, nil, line, hud.ChannelNotice)
	}
	bar.SelectChannel(hud.InputWhisper)
	bar.RememberWhisperer(name, true)
	bar.Whisper.SetText(name)
	c.Input.Focused = bar.Message
}

// hasRadio is FUN_00452fc4: the special slot holds a radio.
func (c *Client) hasRadio() bool {
	items := c.World.Player.Items
	return len(items) >= radioSlot && radioItems[items[radioSlot-1]]
}

// inTeam and inGuild are the memberships the Team and Guild channels
// need (+0x1eff, the guild object's +0xc); teams and guilds are not
// ported, so the player is in neither.
func (c *Client) inTeam() bool  { return c.TeamState != nil && c.TeamState.InParty() }
func (c *Client) inGuild() bool { return false }

// The online players the client knows (PTR_DAT_004c9788, filled by every
// AC4 through FUN_00429a38 whatever its map): whispers find their targets
// and speakers' names here, beyond the map's players.

// rememberPlayer keeps an AC4's ID and name.
func (c *Client) rememberPlayer(id uint32, name []byte) {
	if c.players == nil {
		c.players = map[uint32][]byte{}
	}
	c.players[id] = append([]byte(nil), name...)
}

// playerName is FUN_003c011c: a known player's name, nil when unknown.
func (c *Client) playerName(id uint32) []byte {
	if n := c.World.PeerName(id); n != nil {
		return n
	}
	return c.players[id]
}

// playerByName is FUN_0042a478: the ID of a known player by name, letter
// case ignored.
func (c *Client) playerByName(name []byte) (uint32, bool) {
	if id, ok := c.World.PeerByName(name); ok {
		return id, true
	}
	for id, n := range c.players {
		if bytes.EqualFold(n, name) {
			return id, true
		}
	}
	return 0, false
}

// scrollWheel preserves click-through map input while allowing the transparent
// chat list to scroll under the pointer. Hovered forms and modal dialogs retain
// precedence over the chat behind them.
func (c *Client) scrollWheel(up bool) bool {
	if c.UI.Wheel(up) {
		return true
	}
	if c.Input.Hovered != nil || c.UI.Modal != nil || c.Chat == nil || !c.Chat.Visible || c.Chat.Mode == hud.ChatTickerOnly {
		return false
	}
	if !image.Pt(c.Input.X, c.Input.Y).In(c.Chat.Rect()) {
		return false
	}
	if up {
		c.Chat.WheelUp()
	} else {
		c.Chat.WheelDown()
	}
	return true
}

// whisperBlur validates a typed recipient without stealing focus from the
// control the player just selected. Empty fields leave quietly.
func (c *Client) whisperBlur() {
	if len(c.ChatBar.Whisper.Text) == 0 {
		c.ChatBar.Target = 0
		return
	}
	focused := c.Input.Focused
	c.whisperEnter()
	c.Input.Focused = focused
}
