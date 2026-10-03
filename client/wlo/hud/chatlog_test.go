package hud

import (
	"bytes"
	"testing"
	"time"
)

func newTestLog() (*ChatLog, *time.Time) {
	now := time.Unix(0, 0)
	return &ChatLog{Now: func() time.Time { return now }}, &now
}

// TestChatLogWrap: lines wrap at the list's width (50 characters) and
// take their channel's colour; Local lines read "(Local)name:text" and
// keep their speaker on the first row.
func TestChatLogWrap(t *testing.T) {
	l, _ := newTestLog()
	l.Add(bytes.Repeat([]byte{'a'}, 120), ChannelWorld)
	if len(l.Lines) != 3 || len(l.Lines[0].Text) != 50 || len(l.Lines[2].Text) != 20 {
		t.Fatalf("wrapped into %d rows", len(l.Lines))
	}
	l.Say(7, []byte("Dango"), []byte("hi"), ChannelLocal)
	if last := l.Lines[len(l.Lines)-1]; string(last.Text) != "(Local)Dango:hi" || last.Ink != 0xff80 || last.Speaker != 7 || !last.First {
		t.Fatalf("local line %q %#x", last.Text, last.Ink)
	}
	// A double-byte character is not split across rows.
	l.Lines = nil
	l.Add(append(bytes.Repeat([]byte{'b'}, 49), 0xa4, 0x40, 'c'), ChannelWorld)
	if len(l.Lines[0].Text) != 49 || !bytes.Equal(l.Lines[1].Text, []byte{0xa4, 0x40, 'c'}) {
		t.Fatalf("rows %q %q", l.Lines[0].Text, l.Lines[1].Text)
	}
}

// TestChatTicker: a system line enters the ticker one character per 100 ms
// behind 60 spaces, in red.
func TestChatTicker(t *testing.T) {
	l, now := newTestLog()
	l.Add([]byte("Welcome"), ChannelSystem)
	if l.Lines[0].Ink != 0xf800 {
		t.Fatalf("system ink %#x", l.Lines[0].Ink)
	}
	l.tick()
	if len(l.ticker) != chatTickerLead {
		t.Fatalf("ticker starts %q", l.ticker)
	}
	*now = now.Add(33 * chatTickerEvery)
	l.tick()
	shown := l.ticker[:min(len(l.ticker), chatTickerShown)]
	if lead := len(shown) - len(bytes.TrimLeft(shown, " ")); lead != 27 || !bytes.HasSuffix(shown, []byte("Welcome")) {
		t.Fatalf("after 33 ticks %q (lead %d)", shown, lead)
	}
}

// TestChatPrefixes follows FUN_0048ac28's channels: each speaker line is
// prefix + name + ":" + text; channel 0 is plain with a speaker and
// "(SystemPromp):" without one; channel 10 is plain and red.
func TestChatPrefixes(t *testing.T) {
	l, _ := newTestLog()
	name := []byte("Ann")
	for _, c := range []struct {
		channel int
		speaker uint32
		want    string
	}{
		{ChannelWorld, 1, "(World)Ann:x"},
		{ChannelWhisper, 1, "(Whisp)Ann:x"},
		{ChannelGM, 0, "(GM)Ann:x"},
		{ChannelTeam, 1, "(Team)Ann:x"},
		{ChannelGuild, 1, "(Guild)Ann:x"},
		{ChannelAlly, 1, "(Ally)Ann:x"},
		{ChannelSystem, 1, "x"},
		{ChannelSystem, 0, "(SystemPromp):x"},
		{ChannelNotice, 0, "x"},
		{ChannelPrompt, 0, "(System):x"},
	} {
		l.Say(c.speaker, name, []byte("x"), c.channel)
		if got := string(l.Lines[len(l.Lines)-1].Text); got != c.want {
			t.Errorf("channel %d: %q, want %q", c.channel, got, c.want)
		}
	}
	l.Notice("No target")
	if last := l.Lines[len(l.Lines)-1]; last.Ink != 0xf800 || len(l.queue) != 2 {
		t.Fatalf("notice ink %#x, ticker queue %d", last.Ink, len(l.queue))
	}
}

// TestWhisperInk: whispers are orange (0xfc00, the default ChannelColor2
// 10), as the original shows them.
func TestWhisperInk(t *testing.T) {
	if ink := ChannelInk(ChannelWhisper); ink != 0xfc00 {
		t.Fatalf("whisper ink %#x", ink)
	}
}
