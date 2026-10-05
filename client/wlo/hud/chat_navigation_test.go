package hud

import (
	"bytes"
	"testing"
)

func TestChatScrollLockAndRetention(t *testing.T) {
	l, _ := newTestLog()
	l.SelfID = func() uint32 { return 9 }
	for i := 0; i < 12; i++ {
		l.Say(2, []byte("Ann"), []byte("hello"), ChannelLocal)
	}
	l.WheelUp()
	l.WheelUp()
	l.SetLocked(true)
	first := l.firstRow
	l.Say(2, []byte("Ann"), []byte("new"), ChannelLocal)
	if l.firstRow != first {
		t.Fatal("locked viewport moved")
	}
	l.Say(9, []byte("Me"), []byte("reply"), ChannelLocal)
	if l.Locked || l.firstRow != len(l.Lines)-3 {
		t.Fatal("self reply did not release lock and follow newest")
	}
	for i := 0; i < 200; i++ {
		l.WheelUp()
	}
	l.SetLocked(true)
	for i := 0; i < 200; i++ {
		l.Say(2, []byte("Ann"), []byte("retained"), ChannelLocal)
	}
	if len(l.Lines) != 100 || l.firstRow != 0 {
		t.Fatalf("retention %d, first %d", len(l.Lines), l.firstRow)
	}
	for i := 0; i < 200; i++ {
		l.WheelDown()
	}
	if l.firstRow != 97 {
		t.Fatal("scroll passed newest")
	}
	l.Clear()
	if l.firstRow != 0 || len(l.Lines) != 0 {
		t.Fatal("clear retained history")
	}
}

func TestChatSpeakerRows(t *testing.T) {
	l, _ := newTestLog()
	l.Left, l.Top = 0, 475
	l.Say(7, []byte("Ann"), bytes.Repeat([]byte("x"), 70), ChannelWhisper)
	l.Add([]byte("system"), ChannelPrompt)
	if got := l.SpeakerAt(70, 490); got != 7 {
		t.Fatalf("first row %d", got)
	}
	if got := l.SpeakerAt(70, 510); got != 7 {
		t.Fatalf("continuation row %d", got)
	}
	if got := l.SpeakerAt(70, 530); got != 0 {
		t.Fatalf("system row %d", got)
	}
	for _, p := range [][2]int{{49, 490}, {600, 490}, {70, 600}, {70, 474}} {
		if got := l.SpeakerAt(p[0], p[1]); got != 0 {
			t.Fatalf("outside %v = %d", p, got)
		}
	}
}

func TestChatTransparentHitTest(t *testing.T) {
	l, _ := newTestLog()
	l.Left, l.Top, l.Width, l.Height = 0, 475, 520, 100
	l.SetMode(ChatTransparent)
	if l.HitTest(180, 510) || !l.HitTest(10, 510) {
		t.Fatal("transparent messages swallow map click")
	}
	l.SetMode(ChatOpaque)
	if !l.HitTest(180, 510) {
		t.Fatal("opaque background not interactive")
	}
	l.SetMode(ChatTickerOnly)
	if l.HitTest(10, 510) {
		t.Fatal("hidden list swallows map click")
	}
	l.SetMode(99)
	if l.Mode != ChatTickerOnly {
		t.Fatal("invalid mode accepted")
	}
}

func TestChatEmoticonWrapping(t *testing.T) {
	l, _ := newTestLog()
	l.Add(append(bytes.Repeat([]byte("a"), 49), []byte(":D!")...), ChannelLocal)
	if len(l.Lines) != 2 || len(l.Lines[0].Text) != 49 || string(l.Lines[1].Text) != ":D!" {
		t.Fatalf("code split across rows: %+v", l.Lines)
	}
	// The ASCII trail byte of a Big5 glyph must not start an emoticon.
	if chatUnit([]byte{0xa4, ':'}) != 2 || emoticonAt([]byte{0xa4, ':'}) != -1 {
		t.Fatal("Big5 glyph mistaken for an emoticon")
	}
	if emoticonAt([]byte("?D")) != -1 || emoticonAt([]byte(":D")) != 6 {
		t.Fatal("native code matching changed")
	}
}

func TestChatResizeRewrapAndLockedAnchor(t *testing.T) {
	l, _ := newTestLog()
	l.SetMode(ChatOpaque)
	for i := 0; i < 8; i++ {
		l.Add(bytes.Repeat([]byte("x"), 80), ChannelLocal)
	}
	l.firstRow = 3
	l.SetLocked(true)
	anchor := l.Lines[l.firstRow]
	l.ResizeTo(320, 200)
	current := l.Lines[l.firstRow]
	if current.Entry != anchor.Entry || current.Offset > anchor.Offset || current.Offset+len(current.Text) <= anchor.Offset {
		t.Fatalf("locked message moved: old=%+v new=%+v", anchor, current)
	}
	if l.Width != 320 || l.Height != 200 || l.Top+l.Height != 575 {
		t.Fatal("native resize geometry incorrect")
	}
	if len(l.history) != 8 || len(l.Lines) != 32 {
		t.Fatal("resize lost original messages")
	}
	l.SetMode(ChatTransparent)
	if l.Width != 320 || l.Height != 200 || l.Top != 375 || len(l.Lines) != 32 {
		t.Fatal("transparent mode changed geometry")
	}
	l.SetMode(ChatOpaque)
	if l.Width != 320 || l.Height != 200 {
		t.Fatal("opaque geometry forgotten")
	}
	l.ResizeTo(1, 1)
	if l.Width != 224 || l.Height != 72 {
		t.Fatal("native minima ignored")
	}
	l.ResizeTo(9999, 9999)
	if l.Width != 800 || l.Top != 0 || l.Height != 575 {
		t.Fatal("resize escaped screen")
	}
	l.Clear()
	l.ResizeTo(320, 100)
	if len(l.history) != 0 || len(l.Lines) != 0 {
		t.Fatal("clear left messages to rewrap")
	}
}

func TestChatRetentionKeepsCompleteMessages(t *testing.T) {
	l, _ := newTestLog()
	for i := 0; i < 101; i++ {
		l.Add(bytes.Repeat([]byte("x"), 80), ChannelLocal)
	}
	if len(l.history) != 100 || len(l.Lines) != 200 || !l.Lines[0].First {
		t.Fatal("history trimmed part of a message")
	}
}
