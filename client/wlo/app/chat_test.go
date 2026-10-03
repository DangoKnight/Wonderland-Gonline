package app

import (
	"bytes"
	"encoding/binary"
	"testing"

	"wonderland-go/client/wlo/hud"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// say types a message into the bar and presses Enter.
func say(c *Client, text string) {
	c.ChatBar.Message.SetText([]byte(text))
	c.sendChat()
}

func lastLine(c *Client) string { return string(c.Chat.Lines[len(c.Chat.Lines)-1].Text) }

// TestChatChannels follows the Chat captures: World without a radio,
// Whisper without a target, and Team and Guild without membership are
// refused with a line in the log, and the text stays; Local goes out as
// 2/2, is logged at once and empties the field.
func TestChatChannels(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)

	// Local_Chat_01/02: "(Local)<name>:Test Local", the field empties.
	say(c, "Test Local")
	if lastLine(c) != "(Local)Dango:Test Local" || len(c.ChatBar.Message.Text) != 0 {
		t.Fatalf("local line %q, field %q", lastLine(c), c.ChatBar.Message.Text)
	}
	if l := c.Chat.Lines[len(c.Chat.Lines)-1]; l.Speaker != c.World.Player.ID || l.Channel != hud.ChannelLocal {
		t.Fatalf("local row %+v", l)
	}

	// The button cycles World, Local, Whisper, Team.
	c.ChatBar.Switch.Click() // Local → Whisper
	c.ChatBar.Switch.Click() // Whisper → Team: refused, back to Local
	if c.ChatBar.Channel != hud.InputLocal || lastLine(c) != "(System):You are not in a Team" {
		t.Fatalf("team: channel %d, line %q", c.ChatBar.Channel, lastLine(c))
	}
	c.ChatBar.SelectChannel(hud.InputGuild)
	if c.ChatBar.Channel != hud.InputLocal || lastLine(c) != "(System):You are not in a Guild" {
		t.Fatalf("guild: channel %d, line %q", c.ChatBar.Channel, lastLine(c))
	}

	// World_Chat_02: no radio set.
	c.ChatBar.SelectChannel(hud.InputWorld)
	say(c, "Test World")
	if lastLine(c) != "(System):World Channel requires Radio Set" || string(c.ChatBar.Message.Text) != "Test World" {
		t.Fatalf("world line %q, field %q", lastLine(c), c.ChatBar.Message.Text)
	}

	// Whisper_Chat_02: no name in the whisper field.
	c.ChatBar.Switch.Click() // World → Local
	c.ChatBar.Switch.Click() // Local → Whisper
	if c.ChatBar.Channel != hud.InputWhisper || c.ChatBar.Whisper.ReadOnly {
		t.Fatalf("whisper channel %d, read-only %v", c.ChatBar.Channel, c.ChatBar.Whisper.ReadOnly)
	}
	say(c, "Test Whisper")
	if lastLine(c) != "No target" || string(c.ChatBar.Message.Text) != "Test Whisper" {
		t.Fatalf("whisper line %q, field %q", lastLine(c), c.ChatBar.Message.Text)
	}
	if ink := c.Chat.Lines[len(c.Chat.Lines)-1].Ink; ink != 0xf800 {
		t.Fatalf("notice ink %#x", ink)
	}

	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], append([]byte{protocol.CommandChat, protocol.ChatMapMessage}, "Test Local"...)) {
		t.Fatalf("sent %q", got)
	}
}

// TestChatWhisperAndWorld: a named player on the map becomes the whisper
// target, and the message goes out as 2/3 with the target's ID; with a
// Radio Set in the special slot World sends 2/1 and logs the line.
// Received 2/1 and 2/3 lines carry their channel's prefix.
func TestChatWhisperAndWorld(t *testing.T) {
	c, _, _ := enteredClient(t)
	ann := game.Character{ID: 20002, Slot: 1, Name: "Ann", Level: 3, Element: 2, Body: 1,
		Color1: 444444444, Color2: 444444444, Map: 10017, X: 900, Y: 1100}
	p, err := ann.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(p)
	sent := wire(t, c)

	c.ChatBar.SelectChannel(hud.InputWhisper)
	c.ChatBar.Whisper.SetText([]byte("nobody"))
	c.whisperEnter()
	if lastLine(c) != "<<No such person online>>" || len(c.ChatBar.Whisper.Text) != 0 {
		t.Fatalf("unknown name: %q, field %q", lastLine(c), c.ChatBar.Whisper.Text)
	}
	c.ChatBar.Whisper.SetText([]byte("ann"))
	c.whisperEnter()
	if c.ChatBar.Target != ann.ID || lastLine(c) != "Whisp to <<Ann>>" || string(c.ChatBar.Whisper.Text) != "Ann" {
		t.Fatalf("target %d, line %q, field %q", c.ChatBar.Target, lastLine(c), c.ChatBar.Whisper.Text)
	}
	say(c, "psst")

	c.World.Player.Items = []uint16{0, 0, 0, 0, 0, 34076}
	c.Stats.SP = 20
	c.ChatBar.SelectChannel(hud.InputWorld)
	say(c, "hello all")
	if lastLine(c) != "(World)Dango:hello all" {
		t.Fatalf("world line %q", lastLine(c))
	}

	got := sent()
	whisper := binary.LittleEndian.AppendUint32([]byte{protocol.CommandChat, protocol.ChatWhisperMessage}, ann.ID)
	if len(got) != 2 || !bytes.Equal(got[0], append(whisper, "psst"...)) ||
		!bytes.Equal(got[1], append([]byte{protocol.CommandChat, protocol.ChatWorldMessage}, "hello all"...)) {
		t.Fatalf("sent %q", got)
	}

	c.dispatch(protocol.Builder{protocol.CommandChat, protocol.ChatWorldMessage}.U32(ann.ID).Bytes([]byte("hi")))
	if lastLine(c) != "(World)Ann:hi" {
		t.Fatalf("received world %q", lastLine(c))
	}
	c.dispatch(protocol.Builder{protocol.CommandChat, protocol.ChatWhisperMessage}.U32(ann.ID).Bytes([]byte("psst")))
	if lastLine(c) != "(Whisp)Ann:psst" {
		t.Fatalf("received whisper %q", lastLine(c))
	}
}
