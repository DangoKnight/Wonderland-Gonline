package server

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

func TestWorldChatAndRecipientPreferences(t *testing.T) {
	s, p, w := chatFixture(t)
	settings := p[2].character.Preferences()
	settings.Channels &^= game.ChatChannelWorld
	p[2].character.Settings = &settings
	if err := s.worldCommand(context.Background(), p[0], []byte{2, 1, 'h', 'i'}); err != nil {
		t.Fatal(err)
	}
	want := append([]byte{2, 16, 0, 0, 0, 0}, []byte("(World) Alice: hi")...)
	for _, i := range []int{0, 1} {
		if !hasPacket(w[i].packets(t), want) {
			t.Fatal("world chat not delivered", i)
		}
	}
	if w[2].Len() != 0 {
		t.Fatal("world channel preference ignored")
	}
}

func TestWhisperUsesExactRecipientAndDoesNotLeak(t *testing.T) {
	s, p, w := chatFixture(t)
	say(t, s, p[0], "/whisper Carol hello")
	want := append([]byte{2, 16, 0, 0, 0, 0}, []byte("(Whisper) Alice: hello")...)
	if !hasPacket(w[2].packets(t), want) || w[1].Len() != 0 || w[0].Len() == 0 {
		t.Fatal("whisper leaked or not acknowledged")
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
	if w[0].Len() == 0 || w[2].Len() == 0 || w[1].Len() != 0 {
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
