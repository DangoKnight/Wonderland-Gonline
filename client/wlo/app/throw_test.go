package app

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/minigame"
)

// throwStart is 57/1 for the Hawaii volleyball (template 14304) on the
// Beach scene (variant 50).
var throwStart = []byte{57, 1, minigame.ThrowKind, 0xe0, 0x37, 1, 50}

// TestThrowFlow: 57/1 kind 1 opens the dodgeball game with both sides and
// the Beach background; Leave reports a loss.
func TestThrowFlow(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch(throwStart)
	sp := c.sport
	if sp == nil {
		t.Fatal("no game")
	}
	g, ok := sp.game.(*minigame.Throw)
	if !ok || !sp.start.Visible || g.Background == nil {
		t.Fatalf("game %T background %v", sp.game, g != nil && g.Background != nil)
	}
	sp.start.Start.Click()
	c.Frame()
	if !g.Running() || !sp.exit.Visible {
		t.Fatal("Start did not run the game")
	}
	sp.exit.Exit.OnClick()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{57, 1, 0}) {
		t.Fatalf("sent %x", got)
	}
}

// TestThrowSnapshot renders the game when THROW_SNAPSHOT names a file.
func TestThrowSnapshot(t *testing.T) {
	out := os.Getenv("THROW_SNAPSHOT")
	if out == "" {
		t.Skip("set THROW_SNAPSHOT to an output path")
	}
	c, now, _ := enteredClient(t)
	c.dispatch(throwStart)
	c.Frame()
	savePNG(t, strings.TrimSuffix(out, ".png")+"_start.png", c)
	c.sport.start.Start.Click()
	for range 150 {
		*now = now.Add(30 * time.Millisecond)
		c.Frame()
	}
	savePNG(t, out, c)
}
