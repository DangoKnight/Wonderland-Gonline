package app

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/minigame"
)

// TestMarioFlow: 57/1 kind 11 asks for the balance (34/1/0), 35/4 sets
// it, Start shows the machine's form, its Start button asks the price in
// the sport prompt and then plays 71/11; the server's reply spins the
// reels, and Leave reports a loss with the reward message first.
func TestMarioFlow(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch([]byte{57, 1, minigame.MarioKind, 0x10, 0x27, 1})
	sp := c.sport
	if sp == nil {
		t.Fatal("the fruit machine did not open")
	}
	g, ok := sp.game.(*minigame.Mario)
	if !ok || string(sp.start.price) != "25" || !sp.start.Visible {
		t.Fatalf("game %T price %q", sp.game, sp.start.price)
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{34, 1, 0}) {
		t.Fatalf("balance query %x", got)
	}
	c.dispatch([]byte{35, 4, 40, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if g.Points != 42 {
		t.Fatalf("points %d", g.Points)
	}
	sp.start.Start.Click()
	if !sp.exit.Visible || sp.start.Visible {
		t.Fatal("the machine's form did not show")
	}
	start, exit := sp.exit.Children[1], sp.exit.Exit
	start.Base().OnClick()
	if sp.talk == nil || !sp.talk.Shown() || g.Armed() {
		t.Fatal("no price prompt")
	}
	c.Frame()
	sp.talk.Hide()
	sp.answer(0)
	if !g.Armed() {
		t.Fatal("OK did not arm the machine")
	}
	start.Base().OnClick()
	if got := sent(); len(got) != 2 || !bytes.Equal(got[1], []byte{71, minigame.MarioKind}) {
		t.Fatalf("play %x", got)
	}
	c.dispatch([]byte{71, minigame.MarioKind, 4, 2, 1, 1, 1, 0, 1})
	if !g.Spinning() {
		t.Fatal("the reply did not spin the reels")
	}
	for range 20 {
		*now = now.Add(30 * time.Millisecond)
		c.Frame()
	}
	exit.OnClick() // pending: no prompt, and not finished: still running
	if g.Done() {
		t.Fatal("left during the spin without finishing")
	}
	sp.game.End(false)
	if got := sent(); len(got) != 3 || !bytes.Equal(got[2], []byte{57, 1, 0}) {
		t.Fatalf("result %x", got)
	}
	c.dispatch([]byte{57, 2})
	if c.sport != nil || !c.MainStatus.Visible {
		t.Fatal("57/2 did not close the machine")
	}
}

// TestMarioSnapshot renders the fruit machine's explanation form, a lamp
// win while the reels turn and after they stop, when MARIO_SNAPSHOT names
// an output file.
func TestMarioSnapshot(t *testing.T) {
	out := os.Getenv("MARIO_SNAPSHOT")
	if out == "" {
		t.Skip("set MARIO_SNAPSHOT to an output path")
	}
	c, now, _ := enteredClient(t)
	c.dispatch([]byte{57, 1, minigame.MarioKind, 0x10, 0x27, 1})
	sp := c.sport
	g := sp.game.(*minigame.Mario)
	c.Frame()
	base := strings.TrimSuffix(out, ".png")
	savePNG(t, base+"_start.png", c)
	sp.start.Start.Click()
	g.Points = 100
	g.Confirm = func(_ string, ok func()) { ok() }
	g.Press()
	g.Press()
	c.dispatch([]byte{71, minigame.MarioKind, 4, 2, 3, 3, 3, 1, 1})
	for range 40 {
		*now = now.Add(30 * time.Millisecond)
		c.Frame()
	}
	savePNG(t, base+"_spin.png", c)
	for range 2000 {
		*now = now.Add(30 * time.Millisecond)
		c.Frame()
		if !g.Spinning() {
			break
		}
	}
	savePNG(t, out, c)
}
