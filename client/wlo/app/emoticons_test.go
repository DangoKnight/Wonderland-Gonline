package app

import (
	"bytes"
	"os"
	"testing"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
)

func TestChatEmoticonPickerAndWire(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	b := c.ChatBar
	b.Emotes.Click()
	if !b.EmotePanel.Visible || !b.EmoteClose.Visible || !b.EmoteIcons[30].Visible {
		t.Fatal("picker failed to open")
	}
	b.Message.SetText([]byte("ab"))
	b.Message.KeyDown(seui.VKLeft, 0)
	b.EmoteIcons[6].Click()
	if string(b.Message.Text) != "a:Db" || c.UI.Input.Focused != b.Message {
		t.Fatalf("native code/caret insertion: %q", b.Message.Text)
	}
	// Neither a full message nor a modal dialog may insert part of a code.
	b.Message.SetText(bytes.Repeat([]byte("a"), 59))
	b.EmoteIcons[4].Click()
	if len(b.Message.Text) != 59 {
		t.Fatal("partial emoticon inserted at limit")
	}
	c.UI.Modal = c.Inventory
	b.EmoteIcons[4].Click()
	if len(b.Message.Text) != 59 {
		t.Fatal("picker escaped modal")
	}
	c.UI.Modal = nil
	b.Message.Clear()
	b.EmoteIcons[4].Click()
	b.EmoteIcons[30].Click()
	c.sendChat()
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{2, 2, 'X', 'D', '-', 'P'}) {
		t.Fatalf("native emoticon text packet: %x", got)
	}
	c.Frame()
	if path := os.Getenv("EMOTICON_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
	b.EmoteClose.Click()
	if b.EmotePanel.Visible || b.EmoteIcons[0].Visible {
		t.Fatal("picker failed to close")
	}
	b.ShowEmoticons(true)
	b.Reset()
	if b.EmotePanel.Visible {
		t.Fatal("picker leaked across world entry")
	}
}

func TestChatNativeResizeCapture(t *testing.T) {
	c, _, _ := enteredClient(t)
	l := c.Chat
	l.SetMode(hud.ChatOpaque)
	for i := 0; i < 20; i++ {
		l.Say(1, []byte("Tester"), []byte("A long line of chat to exercise wrapping while the native window changes size."), hud.ChannelLocal)
	}
	c.Frame()
	x, y := l.Resize.Rect().Min.X+2, l.Resize.Rect().Min.Y+2
	c.UI.Input.Hovered = l.Resize
	c.UI.MouseDown(seui.ButtonLeft, 0, x, y)
	if c.UI.Input.Captured != l.Resize {
		t.Fatal("resize handle did not capture pointer")
	}
	c.UI.Input.Hovered = nil
	c.UI.MouseMove(0, x-160, y-120, true)
	c.UI.MouseUp(seui.ButtonLeft, 0, x-160, y-120)
	if c.UI.Input.Captured != nil || l.Width != 360 || l.Height != 220 || l.Top != 355 {
		t.Fatalf("native resize: width=%d height=%d top=%d", l.Width, l.Height, l.Top)
	}
	c.Frame()
	if path := os.Getenv("CHAT_RESIZE_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}

func TestChatTransparentWheelPrecedence(t *testing.T) {
	c, _, _ := enteredClient(t)
	for i := 0; i < 20; i++ {
		c.Chat.Add([]byte("message"), hud.ChannelLocal)
	}
	c.Input.X, c.Input.Y = 200, 510
	c.Input.Hovered = nil
	before := c.Chat.Scroll.Pos
	if !c.scrollWheel(true) || c.Chat.Scroll.Pos != before-1 {
		t.Fatal("transparent text did not scroll")
	}
	c.UI.Modal = c.Inventory
	if c.scrollWheel(true) || c.Chat.Scroll.Pos != before-1 {
		t.Fatal("wheel escaped modal")
	}
	c.UI.Modal = nil
	c.Input.Hovered = c.Inventory
	if c.scrollWheel(true) || c.Chat.Scroll.Pos != before-1 {
		t.Fatal("wheel scrolled behind another form")
	}
	c.Input.Hovered = nil
	c.Input.X, c.Input.Y = 700, 300
	if c.scrollWheel(true) {
		t.Fatal("map wheel scrolled chat")
	}
	c.Input.X, c.Input.Y = 200, 510
	c.Chat.SetMode(hud.ChatTickerOnly)
	if c.scrollWheel(true) {
		t.Fatal("hidden list scrolled")
	}
}

func TestChatEmoticonEditingAndHints(t *testing.T) {
	c, _, _ := enteredClient(t)
	b := c.ChatBar
	e := b.Message
	e.SetText([]byte("a:$:Db"))
	e.KeyDown(seui.VKLeft, 0) // before b
	e.KeyDown(seui.VKLeft, 0) // before :D, skipping both bytes
	if e.Caret != 3 {
		t.Fatal("caret split an emoticon")
	}
	e.KeyDown(seui.VKBack, 0)
	if string(e.Text) != "a:Db" || e.Caret != 1 {
		t.Fatal("backspace failed to remove an entire emoticon")
	}
	e.KeyDown(seui.VKDelete, 0)
	if string(e.Text) != "ab" {
		t.Fatal("delete failed to remove an entire emoticon")
	}
	e.SetText(append([]byte{0xa4, ':'}, []byte("D:D")...))
	e.KeyDown(seui.VKHome, 0)
	e.KeyDown(seui.VKRight, 0)
	if e.Caret != 2 {
		t.Fatal("Big5 trail byte mistaken for an emoticon")
	}
	e.KeyDown(seui.VKRight, 0)
	e.KeyDown(seui.VKRight, 0)
	if e.Caret != 5 {
		t.Fatal("mixed Big5 and emoticons failed caret navigation")
	}
	// The screenshot's hints show the text code, even when the editor renders icons.
	if string(b.EmoteIcons[0].HintText) != "@|" || string(b.EmoteIcons[1].HintText) != ":$" {
		t.Fatal("picker hints lost native codes")
	}
	// Every horizontal-scroll origin and right edge must preserve whole codes.
	e.SetText(bytes.Repeat([]byte(":D"), 30))
	for i := 0; i < 30; i++ {
		e.KeyDown(seui.VKLeft, 0)
		if e.Caret%2 != 0 || e.CaretCol%2 != 0 {
			t.Fatalf("scroll split code: caret=%d column=%d", e.Caret, e.CaretCol)
		}
	}
	for i := 0; i < 30; i++ {
		e.KeyDown(seui.VKRight, 0)
		if e.Caret%2 != 0 || e.CaretCol%2 != 0 {
			t.Fatalf("scroll split code: caret=%d column=%d", e.Caret, e.CaretCol)
		}
	}
	b.ShowEmoticons(true)
	e.SetText([]byte(":$:$"))
	c.Frame()
	if path := os.Getenv("EMOTICON_PREVIEW_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}
