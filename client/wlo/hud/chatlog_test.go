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
// take their channel's colour; Local lines read "(Local)name:text".
func TestChatLogWrap(t *testing.T) {
	l, _ := newTestLog()
	l.Add(bytes.Repeat([]byte{'a'}, 120), ChannelWorld)
	if len(l.Lines) != 3 || len(l.Lines[0].Text) != 50 || len(l.Lines[2].Text) != 20 {
		t.Fatalf("wrapped into %d rows", len(l.Lines))
	}
	l.AddLocal([]byte("Dango"), []byte("hi"))
	if last := l.Lines[len(l.Lines)-1]; string(last.Text) != "(Local)Dango:hi" || last.Ink != 0xff80 {
		t.Fatalf("local line %q %#x", last.Text, last.Ink)
	}
	// A double-byte character is not split across rows.
	l.Lines = nil
	l.Add(append(bytes.Repeat([]byte{'b'}, 49), 0xa4, 0x40, 'c'), ChannelWorld)
	if len(l.Lines[0].Text) != 49 || !bytes.Equal(l.Lines[1].Text, []byte{0xa4, 0x40, 'c'}) {
		t.Fatalf("rows %q", l.Lines)
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
