package app

import (
	"os"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/surface"
)

func TestLongDialogueClickIndicatorLayout(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Talk.Say([]byte("Yes, I did. But I've only been to the islands nearby. I was always sent back by a violent wave every time I had almost reached the open sea. Several years of failure have passed me to give up. By the way, what's your name?"), hud.Speaker{Name: []byte("Robinson"), Face: func(_ *surface.Surface, _, _ int, _ bool) {}}, nil)
	*now = now.Add(time.Second)
	c.Frame()
	_, height := c.Talk.Size()
	_, sheetHeight := c.Pics.Size(c.Pics.Find("Icon_Nextone_1"))
	textBottom := 33 + len(c.Talk.Lines())*18
	if height-25-sheetHeight/2 < textBottom+6 {
		t.Fatal("real indicator picture overlaps dialogue", height, textBottom, sheetHeight)
	}
	if path := os.Getenv("DIALOGUE_INDICATOR_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}
