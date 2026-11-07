package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/mall"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestMallMachinesNativeRequests(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	f := c.Mall
	f.Show()
	f.Tabs[5].Click()
	if f.Pages() != 2 || !f.Games[7].Visible || f.Add[0].Visible {
		t.Fatal("machine layout")
	}
	f.Games[0].Click() // Lucky Wheel is not the unrelated falling-object game.
	if len(sent()) != 0 || !f.Visible {
		t.Fatal("unsupported machine launched another game")
	}
	f.Games[2].Click()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{75, 4, 8}) || !f.Visible {
		t.Fatal("Diamond Cast launch", got)
	}
	f.Show()
	f.Next.Click()
	if !f.Games[1].Visible || f.Games[2].Visible {
		t.Fatal("second machine page")
	}
	f.Games[1].Click()
	if got := sent(); len(got) != 2 || !bytes.Equal(got[1], []byte{75, 4, 6}) {
		t.Fatal("Fortune Egg launch", got)
	}
	f.Show()
	f.Bonus.Click()
	if f.Category != mall.TabHot || f.Tabs[5].Visible || f.Tabs[6].Visible {
		t.Fatal("bonus mall exposes points services")
	}
	c.dispatch([]byte{57, 1, 8, 0, 0, 0})
	if c.sport == nil || c.sport.kind != 8 || !c.sport.start.Visible || len(sent()) != 2 {
		t.Fatal("native game reply did not open instructions, or charged before start")
	}
}

func TestMallReturnsAfterMachineCancelOrExit(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch(mallCatalogFixture())
	c.dispatch([]byte{75, 7, 1})
	f := c.Mall
	f.Show()
	f.Add[0].Click()
	f.Return.Click()
	f.Tabs[5].Click()
	f.Next.Click()
	for _, mode := range []string{"leave", "close", "exit"} {
		f.Games[1].Click() // Page two's Fortune Egg.
		if !f.Visible {
			t.Fatal("mall closed before server entry", mode)
		}
		c.dispatch([]byte{57, 1, 6, 0, 0, 0})
		sp := c.sport
		if sp == nil || !sp.start.Visible || f.Visible {
			t.Fatal("minigame overlay", mode)
		}
		switch mode {
		case "leave":
			sp.start.Leave.Click()
		case "close":
			sp.start.Close.Click()
		case "exit":
			sp.start.Start.Click()
			sp.exit.Exit.OnClick()
		}
		c.dispatch([]byte{57, 2})
		if c.sport != nil || !f.Visible || f.Category != mall.TabGames || f.Page != 2 || len(f.State.Cart) != 1 {
			t.Fatal("mall context not restored", mode)
		}
		c.dispatch([]byte{57, 2}) // A duplicate end must not reset its page/cart.
	}
	for _, p := range sent() {
		if len(p) > 0 && p[0] == 71 {
			t.Fatal("entry/cancellation charged a play", p)
		}
	}
	f.Hide()
	c.dispatch([]byte{57, 1, 6, 0, 0, 0}) // A game launched without the mall must not open it.
	c.sport.start.Leave.Click()
	c.dispatch([]byte{57, 2})
	if f.Visible {
		t.Fatal("unrelated game opened mall")
	}
}

func TestMallForgeNativeReplyAndRetryProtection(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	f := c.Mall
	const fixtureItem = 65000
	c.items[fixtureItem] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 3, Status: [2]uint16{28}, Values: [2]int32{110}}}
	c.InventoryState.Bag[0] = game.Item{ID: fixtureItem, Count: 1}
	f.Confirm = func(_ string, accept func()) { accept() }
	f.Show()
	f.Tabs[6].Click()
	f.LeftUp(0, f.Left+275, f.Top+51)
	if f.ForgeSlot != 1 || !f.ForgeConfirm.Enabled {
		t.Fatal("forge selection")
	}
	f.ForgeConfirm.Click()
	f.ForgeConfirm.Click()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{75, 3, 1}) || c.InventoryState.Bag[0].Forge() != 0 {
		t.Fatal("optimistic or repeated forge", got)
	}
	f.Bonus.Click()
	if f.State.Bonus {
		t.Fatal("currency changed during forge")
	}
	if f.ForgeReply([]byte{75, 6, 1, 2, 1, 0}) || f.ForgeReply([]byte{75, 6, 1, 1, 201, 0}) || !f.ForgePending {
		t.Fatal("invalid receipt unlocked operation")
	}
	*now = now.Add(20 * time.Second)
	f.Update(&seui.Input{})
	if !f.ForgePending || f.ForgeConfirm.Enabled {
		t.Fatal("timeout permitted unsafe retry")
	}
	c.dispatch([]byte{75, 6, 1, 1, 1, 0})
	if f.ForgePending || c.InventoryState.Bag[0].Forge() != 1 || !f.ForgeConfirm.Enabled {
		t.Fatal("late authoritative progress not applied")
	}
	f.ForgeConfirm.Click()
	c.dispatch([]byte{75, 6, 2})
	if f.ForgePending || c.InventoryState.Bag[0].Forge() != 1 {
		t.Fatal("failed roll changed equipment")
	}
	f.ForgeConfirm.Click()
	c.InventoryState.Bag[0] = game.Item{ID: 1, Count: 1}
	if f.ForgeReply([]byte{75, 6, 1, 1, 2, 0}) || c.InventoryState.Bag[0].Forge() != 0 {
		t.Fatal("stale receipt changed replacement item")
	}
}

func TestMallServicesPresentation(t *testing.T) {
	dir := os.Getenv("MALL_SNAPSHOT")
	if dir == "" {
		t.Skip("optional native visual check")
	}
	c, _, _ := enteredClient(t)
	f := c.Mall
	f.Show()
	f.Tabs[5].Click()
	for _, name := range []string{"Icon_Game777_9", "Icon_Game777_2", "Icon_Game777_3", "Icon_Game777_4", "Icon_Game777_5", "Icon_Game777_10", "Icon_Game777_7", "Icon_Game777_8"} {
		if c.Pics.Find(name) < 0 {
			t.Fatal("missing thumbnail")
		}
	}
	c.Frame()
	savePNG(t, filepath.Join(dir, "games.png"), c)
	f.Next.Click()
	c.Frame()
	savePNG(t, filepath.Join(dir, "games-page2.png"), c)
	c.InventoryState.Bag[0] = game.Item{ID: 12101, Count: 1}
	f.Tabs[6].Click()
	f.LeftUp(0, f.Left+275, f.Top+51)
	if c.Pics.Find("Form_ BeatScaffold_1") < 0 {
		t.Fatal("missing forge skin")
	}
	c.Frame()
	savePNG(t, filepath.Join(dir, "forge.png"), c)
}

func TestMallScrollForgeCoveredSlotAndAuthoritativeUpgrade(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	f := c.Mall
	// Use a known native family but a wide footprint to exercise covered cells.
	item := c.items[11101]
	item.Definition.CellWidth, item.Definition.CellHeight = 2, 1
	c.items[11101] = item
	c.InventoryState.Bag[0] = game.Item{ID: 11101, Count: 1}
	c.InventoryState.Bag[4] = game.Item{ID: 30101, Count: 5}
	f.Confirm = func(_ string, accept func()) { accept() }
	f.Show()
	f.Tabs[6].Click()
	f.LeftUp(0, f.Left+310, f.Top+51)
	if f.ForgeSlot != 1 {
		t.Fatal("covered cell did not select its owner")
	}
	f.ForgeConfirm.Click()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{75, 3, 1}) || c.InventoryState.Bag[4].Count != 5 {
		t.Fatal("scroll request changed local inventory", got)
	}
	c.dispatch([]byte{75, 6, 5})
	if f.ForgePending || c.InventoryState.Bag[4].Count != 5 {
		t.Fatal("rejection consumed scrolls")
	}
	f.ForgeConfirm.Click()
	// Inventory delivery precedes the success receipt for family upgrades.
	c.InventoryState.Bag[0] = game.Item{ID: 11103, Count: 1}
	c.InventoryState.Bag[4].Count = 4
	c.dispatch([]byte{75, 6, 6})
	if f.ForgePending || f.ForgeSlot != 0 || c.InventoryState.Bag[0].ID != 11103 || c.InventoryState.Bag[4].Count != 4 {
		t.Fatal("upgrade receipt overwrote inventory delivery")
	}
}
