package app

import (
	"bytes"
	"encoding/binary"
	"testing"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
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

// TestMessageFieldClick: a click on the message field gives it the
// keyboard. It has no picture, so the constructor clears its pixel hit
// test (+0xa8); with it set, the bar took the click instead.
func TestMessageFieldClick(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.Input.X, c.Input.Y = 262, 585
	c.Frame()
	c.UI.MouseDown(seui.ButtonLeft, 0, 262, 585)
	if c.Input.Focused != seui.Control(c.ChatBar.Message) {
		t.Fatalf("focused %T", c.Input.Focused)
	}
}

// faceSpy records the speaker icon's sprite action and frame.
type faceSpy struct{ action, frame, calls int }

func (f *faceSpy) DrawFaceFrame(_ *surface.Surface, _, _, action, frame int) {
	f.action, f.frame = action, frame
	f.calls++
}

func (f *faceSpy) FaceHit(x, y, _, _, px, py int) bool { return px == x && py == y }

// TestChatSpeakerIcon: a speaker's row shows the face sprite's action 4,
// second frame (the turned head of Chat_Icons_02.png); system lines show
// none.
func TestChatSpeakerIcon(t *testing.T) {
	c, _, _ := enteredClient(t)
	spy := &faceSpy{}
	c.Chat.Faces = func(uint32) hud.SpeakerFace { return spy }
	c.Frame()
	if spy.calls != 0 {
		t.Fatal("the welcome line has an icon")
	}
	say(c, "Test")
	c.Frame()
	if spy.calls != 1 || spy.action != 4 || spy.frame != 1 {
		t.Fatalf("icon %+v", spy)
	}
}

// TestChannelButtonOpensList: the pointer over the channel button opens
// the list of the other channels, and a click there leaves the channel
// alone (FUN_00269b30 only cycles while the list is closed).
func TestChannelButtonOpensList(t *testing.T) {
	c, _, _ := enteredClient(t)
	x, y := 0x1f+20, 0x23f+10
	c.Input.X, c.Input.Y = x, y
	c.Frame()
	if !c.ChatBar.Frame.Visible {
		t.Fatal("the list did not open under the pointer")
	}
	c.UI.MouseDown(seui.ButtonLeft, 0, x, y)
	c.UI.MouseUp(seui.ButtonLeft, 0, x, y)
	if c.ChatBar.Channel != hud.InputLocal {
		t.Fatalf("the click switched to channel %d", c.ChatBar.Channel)
	}
	// The list's top button is World (the first other channel).
	c.ChatBar.Others[1].Click()
	if c.ChatBar.Channel != hud.InputWorld {
		t.Fatalf("picked channel %d", c.ChatBar.Channel)
	}
}

// TestWhisperAreaPress: a press on the whisper field's area selects
// Whisper and gives the field the keyboard (FUN_002695d8).
func TestWhisperAreaPress(t *testing.T) {
	c, _, _ := enteredClient(t)
	x, y := 0x49+20, 0x23f+10
	c.Input.X, c.Input.Y = x, y
	c.Frame()
	c.UI.MouseDown(seui.ButtonLeft, 0, x, y)
	if c.ChatBar.Channel != hud.InputWhisper || c.Input.Focused != seui.Control(c.ChatBar.Whisper) {
		t.Fatalf("channel %d, focused %T", c.ChatBar.Channel, c.Input.Focused)
	}
	c.UI.Char('A')
	if string(c.ChatBar.Whisper.Text) != "A" {
		t.Fatalf("whisper field %q", c.ChatBar.Whisper.Text)
	}
}

// TestSpeakerPortraitPress: a press on a speaker's icon whispers to them
// without the "Whisp to" line, and takes the press.
func TestSpeakerPortraitPress(t *testing.T) {
	c, _, _ := enteredClient(t)
	ann := game.Character{ID: 20002, Slot: 1, Name: "Ann", Level: 3, Element: 2, Body: 1,
		Color1: 444444444, Color2: 444444444, Map: 10017, X: 900, Y: 1100}
	p, err := ann.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(p)
	c.dispatch(protocol.Builder{protocol.CommandChat, protocol.ChatMapMessage}.U32(ann.ID).Bytes([]byte("hi")))
	lines := len(c.Chat.Lines)
	spy := &faceSpy{}
	c.Chat.Faces = func(uint32) hud.SpeakerFace { return spy }
	// The newest row's icon: list left − 10, row top + 10.
	c.Input.X, c.Input.Y = 0x32-10, 0x1db+0xe+2*0x14+10
	c.Frame()
	if c.Chat.Hovered != ann.ID {
		t.Fatalf("hovered %d", c.Chat.Hovered)
	}
	if !c.SpeakerPress() {
		t.Fatal("press not taken")
	}
	if c.ChatBar.Target != ann.ID || c.ChatBar.Channel != hud.InputWhisper || string(c.ChatBar.Whisper.Text) != "Ann" || len(c.Chat.Lines) != lines {
		t.Fatalf("target %d, channel %d, field %q, lines %d→%d", c.ChatBar.Target, c.ChatBar.Channel, c.ChatBar.Whisper.Text, lines, len(c.Chat.Lines))
	}
}

// TestWhisperers: another player's whisper joins the recent whisperers;
// on the Whisper channel the list opens under the pointer over the whisper
// field, and picking a name makes it the target.
func TestWhisperers(t *testing.T) {
	c, _, _ := enteredClient(t)
	ann := game.Character{ID: 20002, Slot: 1, Name: "Ann", Level: 3, Element: 2, Body: 1,
		Color1: 444444444, Color2: 444444444, Map: 10017, X: 900, Y: 1100}
	p, err := ann.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(p)
	c.dispatch(protocol.Builder{protocol.CommandChat, protocol.ChatWhisperMessage}.U32(ann.ID).Bytes([]byte("psst")))
	if c.ChatBar.Whisperers.Count() != 1 || string(c.ChatBar.Whisperers.Items[0]) != "Ann" {
		t.Fatalf("whisperers %q", c.ChatBar.Whisperers.Items)
	}
	c.ChatBar.SelectChannel(hud.InputWhisper)
	c.Input.X, c.Input.Y = 0x49+20, 0x23f+10
	c.Frame()
	if !c.ChatBar.WhispererBox.Visible {
		t.Fatal("the whisperers did not open")
	}
	c.ChatBar.Whisperers.OnSelect(0)
	if c.ChatBar.Target != ann.ID || string(c.ChatBar.Whisper.Text) != "Ann" {
		t.Fatalf("target %d, field %q", c.ChatBar.Target, c.ChatBar.Whisper.Text)
	}
	// A player known from an AC4 on another map is still a target by name.
	bob := game.Character{ID: 20003, Slot: 1, Name: "Bob", Level: 3, Element: 2, Body: 1,
		Color1: 444444444, Color2: 444444444, Map: 10003, X: 900, Y: 1100}
	if p, err = bob.AppearancePacket(true); err != nil {
		t.Fatal(err)
	}
	c.dispatch(p)
	c.ChatBar.Whisper.SetText([]byte("bob"))
	c.whisperEnter()
	if c.ChatBar.Target != bob.ID {
		t.Fatalf("target %d, want Bob", c.ChatBar.Target)
	}
}
