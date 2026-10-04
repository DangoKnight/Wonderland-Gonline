package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAdditionalLocalMinigameFlows(t *testing.T) {
	c, now, _ := enteredClient(t)
	wireSent := wire(t, c)
	sent := func() [][]byte {
		var results [][]byte
		for _, p := range wireSent() {
			if len(p) == 3 && p[0] == 57 && p[1] == 1 {
				results = append(results, p)
			}
		}
		return results
	}
	for _, kind := range []byte{2, 5, 13, 15, 17} {
		before := len(sent())
		// Independent raw native AC57 start/hold/end and result packets.
		c.dispatch([]byte{57, 1, kind, 0x10, 0x27, 1})
		c.dispatch([]byte{20, 9})
		sp := c.sport
		if sp == nil || !sp.start.Visible || c.MainStatus.Visible {
			t.Fatal("start lifecycle", kind)
		}
		if got := sent(); len(got) != before {
			t.Fatal("start automatically reported a loss", kind, got)
		}
		sp.start.Start.Click()
		if sp.start.Visible || !sp.game.Running() || !sp.exit.Visible {
			t.Fatal("Start", kind)
		}
		c.GroundClick(350, 300)
		*now = now.Add(time.Second)
		c.Frame()
		sp.exit.Exit.OnClick()
		sp.exit.Exit.OnClick()
		got := sent()[before:]
		if len(got) != 1 || !bytes.Equal(got[0], []byte{57, 1, 0}) {
			t.Fatal("Leave must report once", kind, got)
		}
		c.dispatch([]byte{57, 2})
		if c.sport != nil || !c.MainStatus.Visible {
			t.Fatal("HUD restoration", kind)
		}
	}
}

func TestNativeArcadeDispatchAndCleanup(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch([]byte{57, 1, 6, 0x10, 0x27, 1})
	sp := c.sport
	if sp == nil {
		t.Fatal("egg game did not open")
	}
	if c.Pics.Find("TrunEggBG") < 0 {
		t.Fatal("native JPEG background was not loaded")
	}
	sp.start.Start.Click()
	// Native egg Play button at (410,385), followed by purchase confirmation.
	c.GroundClick(415, 390)
	if len(sent()) != 0 {
		t.Fatal("unconfirmed purchase sent")
	}
	c.GroundClick(415, 390)
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{71, 6, 1}) {
		t.Fatal("purchase", got)
	}
	c.dispatch([]byte{71, 8, 1, 2, 1, 2, 3, 4}) // Another game's reply cannot settle this one.
	c.dispatch([]byte{71, 6, 2})                // Denied; retry must be possible.
	c.GroundClick(415, 390)
	c.GroundClick(415, 390)
	if len(sent()) != 2 {
		t.Fatal("denial did not allow retry")
	}
	c.dispatch([]byte{71, 6, 1, 2, 4})
	*now = now.Add(5 * time.Second)
	sp.game.Update(*now)
	// Purchases never synthesize the event's win/loss result or inventory updates.
	if len(sent()) != 2 {
		t.Fatal("server outcome generated extra packets")
	}
	sp.game.End(false)
	c.dispatch([]byte{57, 2})
	c.dispatch([]byte{71, 6, 1, 2, 4})
	c.sportStartClick(sp, sportTagLeave)
	if c.sport != nil || !c.MainStatus.Visible || len(sent()) != 3 {
		t.Fatal("cleanup or stale callback")
	}
}

// Opt-in renders use the extracted native picture assets, like the existing
// Mole and Hunter snapshots. Nothing is written during ordinary test runs.
func TestAdditionalMinigameSnapshots(t *testing.T) {
	dir := os.Getenv("ARCADE_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("set ARCADE_SNAPSHOT_DIR to an output directory")
	}
	c, now, _ := enteredClient(t)
	for _, kind := range []byte{2, 5, 13, 15, 17, 6, 22, 8, 10, 19} {
		c.dispatch([]byte{57, 1, kind, 0x10, 0x27, 1})
		c.Frame()
		savePNG(t, filepath.Join(dir, fmt.Sprintf("%02d-start.png", kind)), c)
		c.sport.start.Start.Click()
		*now = now.Add(4100 * time.Millisecond)
		c.Frame()
		savePNG(t, filepath.Join(dir, fmt.Sprintf("%02d-play.png", kind)), c)
		c.dispatch([]byte{57, 2})
	}
}
