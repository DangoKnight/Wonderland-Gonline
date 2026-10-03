package hud

import (
	"image"
	"testing"
	"time"
)

// TestTalkAnimation steps FUN_00472664 and FUN_00472868 at 60 frames a
// second: a line opens over two frames at the bare margins, one at full
// width, then full with its text; a closed line shrinks over two frames at
// three quarters of its height and one at the bare size, then hides.
func TestTalkAnimation(t *testing.T) {
	now := time.Unix(0, 0)
	tk := &Talk{Now: func() time.Time { return now }}
	tk.Say([]byte("When the sea wind is blowing, it feels great."), Speaker{}, nil)
	w, h := tk.Size()
	base := image.Pt(2*talkMarginX+talkGrowW, 2*talkMarginY+talkGrowH)
	_, rowH := tk.steps()
	frame := func() (image.Point, bool) {
		tk.tick(now)
		now = now.Add(16 * time.Millisecond)
		return image.Pt(tk.cw, tk.ch), tk.textOn
	}
	for i, want := range []struct {
		size image.Point
		text bool
	}{
		{base, false}, {base, false}, {image.Pt(w, base.Y), false}, {image.Pt(w, h), true}, {image.Pt(w, h), true},
	} {
		if size, text := frame(); size != want.size || text != want.text {
			t.Fatalf("opening frame %d: %v text %v, want %v %v", i, size, text, want.size, want.text)
		}
	}
	if r := tk.rect(); r.Dx() != w || r.Min.X != (talkScreenW-w)/2 || r.Min.Y != talkTop {
		t.Fatalf("open rect %v", r)
	}
	tk.Hide()
	threeQuarters := image.Pt(w, 3*rowH+base.Y)
	for i, want := range []image.Point{threeQuarters, threeQuarters, base} {
		if size, text := frame(); size != want || text {
			t.Fatalf("closing frame %d: %v text %v, want %v", i, size, text, want)
		}
	}
	if frame(); tk.Drawn() {
		t.Fatal("still drawn after closing")
	}
}
