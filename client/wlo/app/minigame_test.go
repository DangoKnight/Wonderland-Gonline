package app

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wonderland-go/client/wlo/cursor"
	"wonderland-go/client/wlo/minigame"
	"wonderland-go/internal/protocol"
)

// moleStart is the server's 57/1 for the cabin's mole machine (type 3,
// default parameter 0x012af8: 11000, the mice) and its 20/9 hold.
var moleStart = []byte{protocol.CommandMinigame, protocol.MinigameStart, 3, 0xf8, 0x2a, 0x01}

// TestMoleFlow: 57/1 hides the HUD and shows the start form; Start runs
// the game with the hammer cursor; Exit gives up and sends 57/1 with 0;
// 57/2 brings the HUD back.
func TestMoleFlow(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch(moleStart)
	c.dispatch([]byte{protocol.CommandEvent, protocol.EventHold})
	sp := c.sport
	if sp == nil {
		t.Fatal("no game")
	}
	if g, ok := sp.game.(moleGame); !ok || g.Variant != minigame.VariantMouse {
		t.Fatalf("not the mice: %+v", sp.game)
	}
	if c.MainStatus.Visible || c.ChatBar.Visible || !sp.start.Visible {
		t.Fatal("start form not shown alone")
	}
	c.Frame()
	sp.start.Start.Click()
	if sp.start.Visible || !sp.exit.Visible || !sp.game.Running() {
		t.Fatal("Start did not run the game")
	}
	c.Frame()
	if got := c.sportCursor(); got != cursor.ShapeHammer {
		t.Fatalf("cursor %d", got)
	}
	// A ground click goes to the game, not to a walk.
	c.GroundClick(400, 400)
	*now = now.Add(minigameSecond)
	c.Frame()
	sp.exit.Exit.OnClick()
	got := sent()
	if len(got) == 0 || !bytes.Equal(got[len(got)-1], []byte{protocol.CommandMinigame, protocol.MinigameResult, 0}) {
		t.Fatalf("Exit sent %x", got)
	}
	c.dispatch([]byte{protocol.CommandMinigame, protocol.MinigameEnd})
	if c.sport != nil || !c.MainStatus.Visible || !c.ChatBar.Visible {
		t.Fatal("57/2 did not restore the HUD")
	}
}

// TestUnportedMinigame: a game type not ported yet answers a loss, so the
// server's event goes on.
func TestUnportedMinigame(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch([]byte{protocol.CommandMinigame, protocol.MinigameStart, 255, 0xf8, 0x2a, 0x01})
	got := sent()
	if c.sport != nil || len(got) != 1 || !bytes.Equal(got[0], []byte{protocol.CommandMinigame, protocol.MinigameResult, 0}) {
		t.Fatalf("sent %x", got)
	}
}

const minigameSecond = time.Second

// TestMoleSnapshot renders the running game for comparison with
// In-Game/Mole_Minigame.png when MOLE_SNAPSHOT names an output file.
func TestMoleSnapshot(t *testing.T) {
	out := os.Getenv("MOLE_SNAPSHOT")
	if out == "" {
		t.Skip("set MOLE_SNAPSHOT to an output path")
	}
	c, now, _ := enteredClient(t)
	c.dispatch(moleStart)
	m := c.sport.game.(moleGame)
	picks := []int{0, 50, 3, 99, 5, 10}
	m.Rand = func(n int) int {
		v := picks[0] % n
		picks = append(picks[1:], picks[0])
		return v
	}
	c.Frame()
	savePNG(t, strings.TrimSuffix(out, ".png")+"_start.png", c)
	c.sport.start.Start.Click()
	for range 200 {
		*now = now.Add(50 * time.Millisecond)
		c.Frame()
	}
	savePNG(t, out, c)
}

func savePNG(t *testing.T, out string, c *Client) {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, c.Screen.RGBA()); err != nil {
		t.Fatal(err)
	}
}

// hunterStart is the server's 57/1 for the cabin's hunting machine (type 4,
// parameter 10000).
var hunterStart = []byte{protocol.CommandMinigame, protocol.MinigameStart, 4, 0x10, 0x27, 0x01}

// TestHunterFlow: 57/1 type 4 shows the hunting form; Start spawns the
// first monster from its NPC template; Leave on a fresh start form sends a
// loss.
func TestHunterFlow(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch(hunterStart)
	sp := c.sport
	if sp == nil {
		t.Fatal("no game")
	}
	g, ok := sp.game.(hunterGame)
	if !ok || !sp.start.Visible || c.MainStatus.Visible {
		t.Fatalf("hunting form not shown: %+v", sp.game)
	}
	if g.NewMonster(minigame.MonsterA) == nil {
		t.Fatal("no monster role from template 17114")
	}
	sp.start.Start.Click()
	for range 10 {
		*now = now.Add(30 * time.Millisecond)
		c.Frame()
	}
	if !g.Running() || g.Lives() != 3 || g.Seconds() != 30 {
		t.Fatalf("running %v lives %d seconds %d", g.Running(), g.Lives(), g.Seconds())
	}
	sp.exit.Exit.OnClick()
	got := sent()
	if len(got) == 0 || !bytes.Equal(got[len(got)-1], []byte{protocol.CommandMinigame, protocol.MinigameResult, 0}) {
		t.Fatalf("Exit sent %x", got)
	}
	c.dispatch([]byte{protocol.CommandMinigame, protocol.MinigameEnd})
	if c.sport != nil || !c.MainStatus.Visible {
		t.Fatal("57/2 did not restore the HUD")
	}
}

// TestHunterSnapshot renders the hunt after 20 seconds when
// HUNTER_SNAPSHOT names an output file.
func TestHunterSnapshot(t *testing.T) {
	out := os.Getenv("HUNTER_SNAPSHOT")
	if out == "" {
		t.Skip("set HUNTER_SNAPSHOT to an output path")
	}
	c, now, _ := enteredClient(t)
	c.dispatch(hunterStart)
	g := c.sport.game.(hunterGame)
	n := 0
	g.Rand = func(k int) int { n = (n*7919 + 13) % 104729; return n % k }
	c.Frame()
	savePNG(t, strings.TrimSuffix(out, ".png")+"_start.png", c)
	c.sport.start.Start.Click()
	c.Input.X, c.Input.Y = 420, 380
	for range 700 {
		*now = now.Add(30 * time.Millisecond)
		c.Frame()
	}
	savePNG(t, out, c)
}
