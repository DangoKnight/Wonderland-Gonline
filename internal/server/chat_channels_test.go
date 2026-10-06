package server

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestWorldChatAndRecipientPreferences(t *testing.T) {
	s, p, w := chatFixture(t)
	settings := p[2].character.Preferences()
	settings.Channels &^= game.ChatChannelWorld
	p[2].character.Settings = &settings
	if err := s.worldCommand(context.Background(), p[0], []byte{2, 1, 'h', 'i'}); err != nil {
		t.Fatal(err)
	}
	// Native 2/1: the speaker's ID and the text; the sending client logs
	// its own line, so it gets no echo.
	want := protocol.Builder{protocol.CommandChat, protocol.ChatWorldMessage}.U32(p[0].character.ID).Bytes([]byte("hi"))
	if !hasPacket(w[1].packets(t), want) {
		t.Fatal("world chat not delivered")
	}
	if w[0].Len() != 0 {
		t.Fatal("world chat echoed to its sender")
	}
	if w[2].Len() != 0 {
		t.Fatal("world channel preference ignored")
	}
	// The /world command echoes, since the sender's client logged it as Local.
	say(t, s, p[0], "/world hey")
	if !hasPacket(w[0].packets(t), protocol.Builder{protocol.CommandChat, protocol.ChatWorldMessage}.U32(p[0].character.ID).Bytes([]byte("hey"))) {
		t.Fatal("/world not echoed")
	}
}

func TestWhisperUsesExactRecipientAndDoesNotLeak(t *testing.T) {
	s, p, w := chatFixture(t)
	say(t, s, p[0], "/whisper Carol hello")
	// Native 2/3 from the sender, to the target and back to the sender.
	want := protocol.Builder{protocol.CommandChat, protocol.ChatWhisperMessage}.U32(p[0].character.ID).Bytes([]byte("hello"))
	if !hasPacket(w[2].packets(t), want) || w[1].Len() != 0 || !hasPacket(w[0].packets(t), want) {
		t.Fatal("whisper leaked or not acknowledged")
	}
	for _, wire := range w {
		wire.Reset()
	}
	// The client's own request names the target by ID.
	if err := s.worldCommand(context.Background(), p[0], protocol.Builder{protocol.CommandChat, protocol.ChatWhisperMessage}.U32(p[2].character.ID).Bytes([]byte("by id"))); err != nil {
		t.Fatal(err)
	}
	want = protocol.Builder{protocol.CommandChat, protocol.ChatWhisperMessage}.U32(p[0].character.ID).Bytes([]byte("by id"))
	if !hasPacket(w[2].packets(t), want) || w[1].Len() != 0 {
		t.Fatal("2/3 by ID not delivered")
	}
	for _, wire := range w {
		wire.Reset()
	}
	say(t, s, p[0], "/whisper Car private")
	if w[2].Len() != 0 || w[1].Len() != 0 {
		t.Fatal("fuzzy recipient leaked private message")
	}
	for _, wire := range w {
		wire.Reset()
	}
	settings := p[2].character.Preferences()
	settings.Channels &^= game.ChatChannelWhisper
	p[2].character.Settings = &settings
	say(t, s, p[0], "/whisper Carol blocked")
	if w[2].Len() != 0 || w[1].Len() != 0 {
		t.Fatal("whisper preference ignored")
	}
}

func TestTeamChatMembershipAndLocalPreferences(t *testing.T) {
	s, p, w := chatFixture(t)
	p[0].party = &party{members: []*Session{p[0], p[2]}}
	say(t, s, p[0], "/team distant party member")
	team := protocol.Builder{protocol.CommandChat, protocol.ChatTeamMessage}.U32(p[0].character.ID).Bytes([]byte("distant party member"))
	if !hasPacket(w[0].packets(t), team) || !hasPacket(w[2].packets(t), team) || w[1].Len() != 0 {
		t.Fatal("party chat scope incorrect")
	}
	for _, wire := range w {
		wire.Reset()
	}
	settings := p[1].character.Preferences()
	settings.Channels &^= game.ChatChannelLocal
	p[1].character.Settings = &settings
	say(t, s, p[0], "local")
	if w[1].Len() != 0 || w[2].Len() != 0 || w[0].Len() != 0 {
		t.Fatal("local filtering or echo incorrect")
	}
}

func TestChatBoundsControlCharactersAndLoading(t *testing.T) {
	s, p, w := chatFixture(t)
	for _, text := range []string{"", strings.Repeat("a", 61), "hello\x00world", "hello\nworld"} {
		say(t, s, p[0], text)
		if w[1].Len() != 0 || w[2].Len() != 0 {
			t.Fatal("invalid chat forwarded", text)
		}
	}
	for _, wire := range w {
		wire.Reset()
	}
	p[0].ready = false
	p[0].warped = true
	say(t, s, p[0], "hello")
	for _, wire := range w {
		if wire.Len() != 0 {
			t.Fatal("loading sender chatted")
		}
	}
	p[0].ready = true
	msg := strings.Repeat("x", 60)
	say(t, s, p[0], msg)
	got := w[1].packets(t)
	if len(got) != 1 || !bytes.Equal(got[0], protocol.Builder{2, 2}.U32(p[0].character.ID).Bytes([]byte(msg))) {
		t.Fatal("boundary chat rejected", got)
	}
}
