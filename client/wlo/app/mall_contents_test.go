package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/client/wlo/mall"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

func TestMallGearFittingDoesNotEquipOrSpend(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	f := c.Mall
	c.Stats.MaxHP = 250
	c.Stats.HP = 329
	c.Stats.Combat = [5]uint16{76, 42, 26, 46, 24}
	beforeGear, beforeBag, beforeStats := c.InventoryState.Equipment, c.InventoryState.Bag, *c.Stats
	f.Show()
	if !f.TryOn(26017) || !f.TryOn(26013) {
		t.Fatal("box fitting unavailable")
	}
	delta := f.FittingDelta()
	if delta.MaxHP != 100 || delta.MAT != 10 || delta.DEF != 12 || delta.MDF != 12 {
		t.Fatal("captured fitting bonuses", delta, f.Fitted)
	}
	c.Frame()
	if path := os.Getenv("MALL_SNAPSHOT"); path != "" {
		savePNG(t, filepath.Join(path, "fitting.png"), c)
	}
	if c.InventoryState.Equipment != beforeGear || c.InventoryState.Bag != beforeBag || *c.Stats != beforeStats || len(sent()) != 0 {
		t.Fatal("fitting mutated authoritative state")
	}
	// A replacement in the same slot must not stack bonuses.
	f.TryOn(26012)
	delta = f.FittingDelta()
	if delta.ATK != 12 || delta.DEF != 0 || delta.MDF != 0 || delta.MAT != 22 {
		t.Fatal("slot replacement", delta)
	}
	f.ResetPreview.Click()
	if f.Fitted != (game.Equipment{}) || f.FittingDelta() != (game.Combat{}) {
		t.Fatal("reset retained trial gear")
	}
}

func TestMallTabsUseNativePreviewPoses(t *testing.T) {
	c, _, _ := enteredClient(t)
	f := c.Mall
	f.Show()
	drawPreview := f.Preview
	for i, want := range []int{13, 13, 17, 51, 51, 13, 51} {
		f.Tabs[i].Click()
		if got := f.PreviewAction(); got != want {
			t.Fatalf("tab %d pose %d, want %d", i+1, got, want)
		}
		action := -1
		f.Preview = func(x, y, pose int) { action = pose; drawPreview(x, y, pose) }
		c.Frame()
		if path := os.Getenv("MALL_SNAPSHOT"); path != "" {
			savePNG(t, filepath.Join(path, fmt.Sprintf("pose-tab-%d.png", i+1)), c)
		}
		if action != want {
			t.Fatal("renderer received wrong pose", action, want)
		}
		f.ResetPreview.Click()
		if f.PreviewAction() != want {
			t.Fatal("reset lost tab pose")
		}
	}
}

func TestMallBagContentsSelectionAndLifecycle(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	f := c.Mall
	f.State.Catalog[0] = []mall.Entry{{ID: 55134, Count: 1, Cost: 48, Discount: 100, Badge: 1, Category: 3, Order: 3}}
	f.Show()
	f.LeftUp(0, f.Left+250, f.Top+45)
	p := f.ContentsForm
	if !p.Visible || p.ItemID != 55134 || len(p.Rewards) != 26 || p.Width != 467 || p.Top != 60 || c.UI.Modal != p {
		t.Fatal("missing native contents", p)
	}
	if len(sent()) != 0 || len(f.State.Cart) != 0 {
		t.Fatal("preview sent purchase")
	}
	f.Add[0].Click()
	if len(f.State.Cart) != 0 || f.InCart {
		t.Fatal("modal preview allowed underlying purchase selection")
	}
	c.Frame()
	if path := os.Getenv("MALL_SNAPSHOT"); path != "" {
		savePNG(t, filepath.Join(path, "contents.png"), c)
	}
	if !c.MallKey(escapeKey) || p.Visible || !f.Visible || c.UI.Modal != nil {
		t.Fatal("escape did not close only preview")
	}
	f.ShowContents(55134)
	p.Cancel.Click()
	if p.Visible || c.UI.Modal != nil {
		t.Fatal("cancel")
	}
	f.ShowContents(55134)
	p.Close.Click()
	if p.Visible || c.UI.Modal != nil {
		t.Fatal("close")
	}
	f.ShowContents(55134)
	f.Hide()
	if p.Visible || c.UI.Modal != nil {
		t.Fatal("hidden parent retained preview")
	}
	if f.ShowContents(1) {
		t.Fatal("invented contents")
	}
}

func TestMallCompanionEggPortraitPresentation(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.Mall.Show()
	if !c.Mall.ShowContents(34172) {
		t.Fatal("missing egg")
	}
	if name := c.Mall.RewardName(30240); name == "" {
		t.Fatal("voucher companion has no name")
	}
	c.Frame()
	if path := os.Getenv("MALL_SNAPSHOT"); path != "" {
		savePNG(t, filepath.Join(path, "companion-egg.png"), c)
	}
	c.Mall.Reset()
	if c.UI.Modal != nil || c.Mall.Fitted != (game.Equipment{}) {
		t.Fatal("reset retained fitting/modal")
	}
}

func TestMallContentsDragAndOutsideDismiss(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.Mall.Show()
	c.Mall.ShowContents(55134)
	p := c.Mall.ContentsForm
	left, top := p.Left, p.Top
	c.Input.X, c.Input.Y = left+70, top+16
	c.Frame()
	if !c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y) || !p.Dragging {
		t.Fatal("header did not start drag")
	}
	c.UI.MouseMove(0, left+30, top+41, true)
	if p.Left != left-40 || p.Top != top+25 {
		t.Fatal("drag position", p.Left, p.Top)
	}
	c.Frame()
	c.UI.MouseUp(seui.ButtonLeft, 0, 0, 0)
	if p.Dragging || c.Input.Captured != nil || !p.Visible {
		t.Fatal("drag release")
	}
	// An outside click over a catalog tab dismisses only; it must not select it.
	tab := c.Mall.Tabs[1].Abs()
	c.Input.X, c.Input.Y = tab.X+15, tab.Y+8
	c.Frame()
	if !c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y) || p.Visible || c.UI.Modal != nil {
		t.Fatal("outside click did not dismiss")
	}
	c.Frame()
	c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if c.Mall.Category != mall.TabHot || c.Input.Captured != nil {
		t.Fatal("outside click reached catalog")
	}
	c.Mall.ShowContents(55134)
	p = c.Mall.ContentsForm
	c.Input.X, c.Input.Y = p.Left+75, p.Top+80
	c.Frame()
	c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if p.Dragging || !p.Visible {
		t.Fatal("contents body initiated drag or dismissed")
	}
	c.UI.MouseUp(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
}

func TestMallChobitsContainerPreviews(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.Mall.Show()
	for _, id := range []uint16{35450, 35451, 56276} {
		if !c.Mall.ShowContents(id) || len(c.Mall.ContentsForm.Rewards) != 3 {
			t.Fatal("missing container preview", id)
		}
		if (id == 56276) != c.Mall.IncompleteContents[id] {
			t.Fatal("partial preview metadata", id)
		}
		c.Frame()
		if path := os.Getenv("MALL_SNAPSHOT"); path != "" {
			savePNG(t, filepath.Join(path, fmt.Sprintf("container-%d.png", id)), c)
		}
		c.Mall.ContentsForm.Hide()
	}
	if len(sent()) != 0 {
		t.Fatal("preview sent a purchase or consumed an item")
	}
}
