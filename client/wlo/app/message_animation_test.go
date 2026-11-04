package app

import (
	"image"
	"os"
	"testing"
	"time"
)

func TestBattleResultsMessageAnimation(t *testing.T) {
	c, now := resultsClient(t)
	c.dispatch([]byte{8, 1, 36, 1, 36, 0, 0, 0, 0, 0, 0, 0})
	closeResultBattle(c)
	showResultAfterFade(c, now)
	f := c.battleResults.form
	if f == nil || !f.Visible || len(f.list.Items) == 0 {
		t.Fatal("results content delayed until after animation")
	}
	target := f.Rect()
	if f.MessageRect() != target.Inset(8) {
		t.Fatal("results did not use dialogue opening", f.MessageRect(), target)
	}
	until := c.battleResults.until
	*now = now.Add(100 * time.Millisecond)
	c.Frame()
	if f.MessageRect() != target || f.Rect() != target || c.battleResults.until != until {
		t.Fatal("animation changed layout or extended the results timeout")
	}
	*now = until
	c.Frame()
	if f.Visible {
		t.Fatal("timeout left an empty results frame")
	}
	if path := os.Getenv("MESSAGE_CLOSE_SNAPSHOT"); path != "" {
		*now = now.Add(25 * time.Millisecond)
		c.Frame()
		savePNG(t, path, c)
	}
}

func TestTimedNoticeMessageAnimation(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Notices.Show([]byte("Item obtained"), time.Second, *now)
	c.Notices.Draw(c.Env, *now)
	p := c.Notices.panel
	target := p.Rect()
	// Decoration is animated; the notice text is present from its first frame.
	yellow := false
	for y := noticeTop + noticeTextY; y < noticeTop+noticeTextY+noticeLineH; y++ {
		for x := target.Min.X; x < target.Max.X; x++ {
			yellow = yellow || c.Screen.Pix[y*ScreenWidth+x] == noticeInk
		}
	}
	if !yellow || target.Empty() || !c.Notices.shown {
		t.Fatal("notice opening delayed its text")
	}
	*now = now.Add(time.Second)
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	c.Notices.Draw(c.Env, *now)
	for _, pixel := range c.Screen.Pix {
		if pixel != 0 {
			t.Fatal("expired notice left its panel visible")
		}
	}
}

func TestNoticeClosingHasNoMissingFrame(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Notices.Show([]byte("Obtain item"), time.Second, *now)
	c.Frame()
	until := c.Notices.until
	for _, ms := range []int{-1, 0, 16, 32} {
		*now = until.Add(time.Duration(ms) * time.Millisecond)
		c.Frame()
		text := false
		for y := noticeTop + noticeTextY - 5; y < noticeTop+noticeTextY+noticeLineH+5; y++ {
			for x := 300; x < 500; x++ {
				text = text || c.Screen.Pix[y*ScreenWidth+x] == noticeInk
			}
		}
		if !text {
			t.Fatalf("notification vanished before closing completed: %d ms", ms)
		}
	}
}
