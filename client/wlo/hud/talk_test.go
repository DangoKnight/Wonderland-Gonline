package hud

import (
	"image"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/surface"
)

func TestTalkNextIndicatorBelowLongText(t *testing.T) {
	text := []byte("Yes, I did. But I've only been to the islands nearby. I was always sent back by a violent wave every time I had almost reached the open sea. Several years of failure have passed me to give up. By the way, what's your name?")
	for _, who := range []Speaker{{}, {Face: func(_ *surface.Surface, _, _ int, _ bool) {}}, {Art: func(_ *surface.Surface, _, _ int, _ bool) {}}} {
		tk := &Talk{Now: time.Now}
		tk.Say(text, who, nil)
		textY := talkTextY
		if tk.big {
			textY = talkBigTextY
		}
		bottom := textY + len(tk.lines)*talkLineH
		indicatorTop := tk.h - talkNextBottom - talkLineH
		if indicatorTop < bottom+talkNextTextGap {
			t.Fatal("indicator overlaps dialogue", bottom, indicatorTop)
		}
	}
}

func TestTalkAnimation(t *testing.T) {
	for _, speaker := range []Speaker{{}, {Face: func(_ *surface.Surface, _, _ int, _ bool) {}}, {Art: func(_ *surface.Surface, _, _ int, _ bool) {}}} {
		now := time.Unix(0, 0)
		tk := &Talk{Now: func() time.Time { return now }}
		tk.Say([]byte("When the sea wind is blowing, it feels great."), speaker, nil)
		w, h := tk.Size()
		if !tk.Shown() || !tk.textOn {
			t.Fatal("opening animation delayed the content")
		}
		previous := image.Pt(tk.cw, tk.ch)
		for _, ms := range []int{1, 17, 43, 79, 100, 130} {
			now = time.Unix(0, 0).Add(time.Duration(ms) * time.Millisecond)
			tk.tick(now)
			if tk.cw < previous.X || tk.ch < previous.Y || tk.cw > w || tk.ch > h {
				t.Fatal("opening size jumped or overshot", tk.cw, tk.ch)
			}
			previous = image.Pt(tk.cw, tk.ch)
		}
		if tk.cw != w || tk.ch != h {
			t.Fatal("opening did not finish on time")
		}
		tk.Hide()
		if tk.Shown() || tk.textOn || tk.cw != w || tk.ch != h {
			t.Fatal("close delayed dismissal or snapped the panel")
		}
		if tk.Drawn() || tk.Contains(tk.origin.X+w/2, tk.origin.Y+h/2) {
			t.Fatal("dismissal left an empty panel visible or intercepting input")
		}
		tk.Draw() // No environment is needed: there is no panel left to paint.
		now = now.Add(40 * time.Millisecond)
		tk.tick(now)
		if tk.cw > w || tk.cw < w-16 || tk.ch > h || tk.ch < h-16 {
			t.Fatal("closing collapsed instead of using a small transition")
		}
		tk.Hide() // A later server event-end must not restart the timer.
		now = now.Add(40 * time.Millisecond)
		if tk.Drawn() || tk.Contains(tk.origin.X+w/2, tk.origin.Y+h/2) {
			t.Fatal("closed panel lingered or intercepted input")
		}
	}
}

func TestTalkTransitionCanBeInterrupted(t *testing.T) {
	now := time.Unix(0, 0)
	tk := &Talk{Now: func() time.Time { return now }}
	tk.Say([]byte("First line"), Speaker{}, nil)
	now = now.Add(20 * time.Millisecond)
	tk.Hide() // Dismiss before the opening finishes.
	now = now.Add(15 * time.Millisecond)
	tk.tick(now)
	w, h := tk.cw, tk.ch
	tk.Say([]byte("The next line is ready"), Speaker{}, nil)
	if tk.cw != w || tk.ch != h || !tk.textOn || !tk.Shown() {
		t.Fatal("next line waited for a close or reset to a collapsed panel")
	}
	now = now.Add(time.Second) // Catch up after skipped rendering.
	if !tk.Drawn() || tk.cw != tk.w || tk.ch != tk.h {
		t.Fatal("opening depended on the number of rendered frames")
	}
	tk.Hide()
	now = now.Add(time.Second)
	if tk.Drawn() {
		t.Fatal("closing depended on the number of rendered frames")
	}
}
