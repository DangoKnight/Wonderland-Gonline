package app

import (
	"image"
	"os"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

func TestChatReferenceGeometryAndDrag(t *testing.T) {
	c, now, _ := enteredClient(t)
	l := c.Chat
	l.SetMode(hud.ChatOpaque)
	l.ResizeTo(592, 450)
	if l.Top != 125 || l.Top+l.Height != 575 {
		t.Fatal("background mode displaced the HUD bottom")
	}
	c.Frame()
	// Native 32X32Grid must overlay the scene inside the background frame.
	original := append([]uint16(nil), c.Screen.Pix...)
	l.SetMode(hud.ChatTransparent)
	c.Frame()
	changed := 0
	for y := 180; y < 240; y++ {
		for x := 100; x < 200; x++ {
			if c.Screen.Pix[y*800+x] != original[y*800+x] {
				changed++
			}
		}
	}
	if changed < 100 {
		t.Fatal("background grid missing")
	}
	before := l.Rect()
	for _, mode := range []hud.ChatMode{hud.ChatTickerOnly, hud.ChatOpaque, hud.ChatTransparent, hud.ChatOpaque} {
		l.SetMode(mode)
		if l.Rect() != before {
			t.Fatal("mode switch discarded geometry")
		}
	}
	c.Input.Hovered = l
	c.UI.MouseDown(seui.ButtonLeft, 0, 80, 160)
	c.Input.Hovered = nil
	c.UI.MouseMove(0, 200, 110, true)
	// Render while the pointer is away from the form: capture must survive.
	c.Frame()
	c.UI.MouseUp(seui.ButtonLeft, 0, 200, 110)
	if l.Left != 120 || l.Top != 75 || c.Input.Captured != nil {
		t.Fatalf("drag failed: %v", l.Rect())
	}
	moved := l.Rect()
	l.SetLocked(true)
	c.Input.Hovered = l
	c.UI.MouseDown(seui.ButtonLeft, 0, 200, 110)
	c.UI.MouseMove(0, 300, 200, true)
	c.UI.MouseUp(seui.ButtonLeft, 0, 300, 200)
	if l.Rect() != moved {
		t.Fatal("locked chat moved")
	}
	l.SetLocked(false)
	// The ticker stays inside the window and moves with it.
	*now = now.Add(33 * 100 * time.Millisecond) // native welcome ticker after 33 ticks
	c.Frame()
	if path := os.Getenv("CHAT_REFERENCE_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
	l.SetMode(hud.ChatTransparent)
	if l.Rect() != moved || !l.HitTest(125, 200) || l.HitTest(300, 200) {
		t.Fatal("moved transparent mode lost geometry or click-through")
	}
	if !image.Pt(l.Left+50, l.Top+l.Height-20).In(l.Rect()) {
		t.Fatal("ticker outside window")
	}
}

func TestChatWhisperBlurValidation(t *testing.T) {
	c, _, _ := enteredClient(t)
	b := c.ChatBar
	b.SelectChannel(hud.InputWhisper)
	b.Whisper.SetText([]byte("Unknown"))
	b.Target = 123
	c.UI.Focus(b.Whisper)
	c.Frame()
	c.Input.Hovered = b.Message
	c.UI.MouseDown(seui.ButtonLeft, 0, 200, 580)
	c.Frame()
	if lastLine(c) != "<<No such person online>>" || len(b.Whisper.Text) != 0 || b.Target != 0 || c.Input.Focused != b.Message {
		t.Fatalf("unknown recipient blur: text=%q target=%d line=%q", b.Whisper.Text, b.Target, lastLine(c))
	}
	lines := len(c.Chat.Lines)
	c.Frame()
	if len(c.Chat.Lines) != lines {
		t.Fatal("blur warning repeated")
	}
	ann := game.Character{ID: 20002, Slot: 1, Name: "Ann", Level: 3, Element: 2, Body: 1, Map: 10017, X: 900, Y: 1100}
	p, err := ann.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(p)
	b.Whisper.SetText([]byte("ann"))
	c.UI.Focus(b.Whisper)
	c.Frame()
	c.UI.Focus(c.Inventory)
	c.Frame()
	if b.Target != ann.ID || string(b.Whisper.Text) != "Ann" || c.Input.Focused != c.Inventory {
		t.Fatal("valid blur failed to resolve recipient or stole focus")
	}
	b.Whisper.Clear()
	c.UI.Focus(b.Whisper)
	c.Frame()
	lines = len(c.Chat.Lines)
	c.UI.Focus(b.Message)
	c.Frame()
	if len(c.Chat.Lines) != lines || b.Target != 0 {
		t.Fatal("empty blur produced a warning or retained target")
	}
}
