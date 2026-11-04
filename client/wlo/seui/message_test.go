package seui

import (
	"image"
	"testing"
	"time"
)

func TestMessageOpeningGeometryAndControls(t *testing.T) {
	now := time.Unix(0, 0)
	f := NewForm(testEnv())
	f.Env.UI.Add(f)
	f.Left, f.Top, f.Width, f.Height = 250, 30, 300, 150
	f.AnimateMessage(func() time.Time { return now })
	target := f.Rect()
	f.Show()
	if f.MessageRect() != target.Inset(8) || f.Rect() != target || !f.Visible {
		t.Fatal("opening changed layout or delayed visibility")
	}
	previous := f.MessageRect()
	for _, ms := range []int{1, 17, 50, 99, 100, 250} {
		now = time.Unix(0, 0).Add(time.Duration(ms) * time.Millisecond)
		r := f.MessageRect()
		centerDelta := r.Min.Add(r.Max).Sub(target.Min.Add(target.Max))
		if !previous.In(r) || !r.In(target) || centerDelta.X < -1 || centerDelta.X > 1 || centerDelta.Y < -1 || centerDelta.Y > 1 {
			t.Fatal("opening jumped, overshot or lost its center", r)
		}
		previous = r
	}
	if f.MessageRect() != target || f.Rect() != target {
		t.Fatal("opening did not reach final bounds independently of frame count")
	}
	f.Show()
	if f.MessageRect() != target {
		t.Fatal("showing an already visible message restarted its opening")
	}
	f.Hide()
	if f.Visible {
		t.Fatal("dismissal retained an empty form")
	}
	f.Show()
	if f.MessageRect() != target.Inset(8) {
		t.Fatal("reopened message did not animate")
	}
	// Module windows have no message motion unless explicitly enabled.
	plain := NewForm(testEnv())
	plain.Left, plain.Top, plain.Width, plain.Height = 250, 30, 300, 150
	if plain.MessageRect() != image.Rect(250, 30, 550, 180) {
		t.Fatal("ordinary module window animated")
	}
}

func TestMessageClosingKeepsContentAndExpires(t *testing.T) {
	env := testEnv()
	now := time.Unix(0, 0)
	clock := func() time.Time { return now }
	owner := new(int)
	paint := func() {
		env.Screen.Fill(image.Rect(100, 100, 240, 180), 0x1234)
		env.Screen.Fill(image.Rect(130, 120, 210, 140), 0xffe0)
	}
	env.UI.CloseMessage(owner, clock, paint)
	for _, elapsed := range []time.Duration{0, 25 * time.Millisecond} {
		now = time.Unix(0, 0).Add(elapsed)
		env.Screen.Fill(image.Rect(0, 0, env.Screen.W, env.Screen.H), 0)
		env.UI.DrawMessageExits()
		panel, text := false, false
		for _, pixel := range env.Screen.Pix {
			panel = panel || pixel == 0x1234
			text = text || pixel == 0xffe0
		}
		if !panel || !text {
			t.Fatal("closing lost message contents or only drew an empty panel")
		}
	}
	now = now.Add(25 * time.Millisecond)
	env.Screen.Fill(image.Rect(0, 0, env.Screen.W, env.Screen.H), 0)
	env.UI.DrawMessageExits()
	if len(env.UI.messageExits) != 0 {
		t.Fatal("closing extended past 50 ms")
	}
	for _, pixel := range env.Screen.Pix {
		if pixel != 0 {
			t.Fatal("expired closing still drew")
		}
	}
	env.UI.CloseMessage(owner, clock, paint)
	env.UI.CancelMessageExit(owner)
	if len(env.UI.messageExits) != 0 {
		t.Fatal("replacement message retained the old closing image")
	}
}

func TestOverlayMessageClosingPreservesDitherPixels(t *testing.T) {
	env := testEnv()
	now := time.Unix(0, 0)
	env.UI.CloseOverlayMessage(new(int), func() time.Time { return now }, func() {
		for y := 100; y < 140; y++ {
			for x := 100; x < 140; x++ {
				if (x+y)%2 == 0 {
					env.Screen.Fill(image.Rect(x, y, x+1, y+1), 0x1234)
				}
			}
		}
	})
	env.UI.DrawMessageExits()
	for _, pixel := range env.Screen.Pix {
		if pixel != 0 {
			t.Fatal("overlay rendered behind ordinary forms")
		}
	}
	env.UI.DrawOverlayMessageExits()
	before := append([]uint16(nil), env.Screen.Pix...)
	now = now.Add(25 * time.Millisecond)
	env.Screen.Fill(image.Rect(0, 0, env.Screen.W, env.Screen.H), 0)
	env.UI.DrawOverlayMessageExits()
	for y := 0; y < env.Screen.H; y++ {
		for x := 0; x < env.Screen.W; x++ {
			want := uint16(0)
			if y+3 < env.Screen.H {
				want = before[(y+3)*env.Screen.W+x]
			}
			if env.Screen.Pix[y*env.Screen.W+x] != want {
				t.Fatal("closing resampled the dither mask")
			}
		}
	}
	env.UI.CancelMessageExit(nil)
}
