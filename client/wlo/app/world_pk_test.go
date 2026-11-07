package app

import (
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/worldpk"
)

func TestWorldPKPresentationOnlyTabsAndSettings(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	f := c.WorldPK
	if f == nil || f.Visible {
		t.Fatal("World PK should start closed")
	}
	c.FuncButtons.Buttons[worldPKButton].Click()
	if !f.Visible || f.Left != 222 || f.Top != 53 || f.Width != 356 || f.Height != 493 {
		t.Fatal("native World PK placement", f.Left, f.Top, f.Width, f.Height)
	}
	if f.Tab != worldpk.PVP || !f.Start.Visible || f.Start.Enabled || !f.ViewRanking.Visible || len(f.State.Entries) != 0 {
		t.Fatal("invalid preview state")
	}
	f.Start.Click()
	f.ViewRanking.Click()
	if f.Tab != worldpk.Rankings || f.Start.Visible || f.Allow.Visible || !f.MyRanking.Visible {
		t.Fatal("ranking tab visibility")
	}
	f.Tabs[worldpk.Settings].Click()
	if !f.Allow.Visible || f.Next.Visible || f.Confirm.Visible {
		t.Fatal("settings tab visibility")
	}
	f.Disallow.Click()
	if !f.State.AllowOffline || !f.Confirm.Visible || !f.Cancel.Visible {
		t.Fatal("settings draft applied early")
	}
	f.Cancel.Click()
	if !f.State.AllowOffline || f.Confirm.Visible {
		t.Fatal("settings cancel failed")
	}
	f.Disallow.Click()
	f.Confirm.Click()
	if f.State.AllowOffline || f.Confirm.Visible {
		t.Fatal("local settings preview not applied")
	}
	f.Tabs[worldpk.PVP].Click()
	if len(sent()) != 0 {
		t.Fatal("UI preview sent requests", sent())
	}
	if !c.WorldPKKey(escapeKey) || f.Visible {
		t.Fatal("escape did not close World PK")
	}
	c.FuncButtons.Buttons[worldPKButton].Click()
	f.CloseCorner.Click()
	if f.Visible {
		t.Fatal("corner close failed")
	}
	if out := os.Getenv("WORLD_PK_SNAPSHOT"); out != "" {
		f.State.Entries = make([]worldpk.Entry, 300)
		for i := range f.State.Entries {
			f.State.Entries[i] = worldpk.Entry{Name: []byte("Preview"), Level: 190, Wins: uint32(26135 - i)}
		}
		for i, sprite := range []int{3602, 3603, 3604, 3605} {
			if face, err := c.lib.Image("003f", sprite, 2, 0); err == nil {
				f.State.Entries[i].Portrait = surface.FromImage(face)
			}
		}
		f.State.AllowOffline = true
		f.Show()
		for _, tab := range []int{worldpk.PVP, worldpk.Rankings, worldpk.Settings} {
			f.SelectTab(tab)
			c.Frame()
			savePNG(t, filepath.Join(out, []string{"pvp.png", "rankings.png", "settings.png"}[tab]), c)
		}
	}
}

func TestWorldPKRankingPageBoundsAndLifecycle(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	f := c.WorldPK
	f.State.Entries = make([]worldpk.Entry, 301)
	f.Show()
	f.Tabs[worldpk.Rankings].Click()
	f.Last.Click()
	if f.Page != 75 || f.Next.Enabled || f.Last.Enabled {
		t.Fatal("Top 300 cap missing", f.Page)
	}
	f.Previous.Click()
	if f.Page != 74 {
		t.Fatal("previous page")
	}
	f.First.Click()
	if f.Page != 1 || f.Previous.Enabled {
		t.Fatal("first page")
	}
	f.State.Rank = 299
	f.Refresh()
	f.MyRanking.Click()
	if f.Page != 75 {
		t.Fatal("my rank page")
	}
	f.Close.Click()
	if f.Visible {
		t.Fatal("bottom close failed")
	}
	f.Show()
	c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
	if f.Visible {
		t.Fatal("World PK remained open over battle")
	}
	f.Reset()
	if len(f.State.Entries) != 0 || f.Tab != worldpk.PVP || f.Page != 1 || f.Visible {
		t.Fatal("reset retained session state")
	}
}
