package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/mall"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func mallCatalogFixture() []byte {
	return []byte{75, 1, 2, 0, 154, 152, 1, 32, 0, 100, 1, 5, 1, 0, 155, 152, 1, 48, 0, 100, 0, 4, 2, 0}
}

func TestMallNativeCatalogCartAndCheckout(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.dispatch(mallCatalogFixture())
	c.dispatch([]byte{75, 7, 1})
	c.FuncButtons.Mall.Click()
	f := c.Mall
	if !f.Visible || f.Width != 778 || f.Height != 554 || f.Left != 11 || f.Top != 23 {
		t.Fatal("native placement", f.Left, f.Top, f.Width, f.Height)
	}
	f.Add[0].Click()
	if !f.InCart || len(f.State.Cart) != 1 || f.Width != 548 || f.Height != 534 {
		t.Fatal("native cart")
	}
	f.Quantities[0].SetText([]byte("2"))
	f.Quantities[0].OnChange()
	f.Return.Click()
	f.ViewCart.Click()
	if len(f.State.Cart) != 1 || f.State.Cart[0].Quantity != 2 {
		t.Fatal("cart lost on return")
	}
	f.Pay.Click()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{34, 1, 1}) {
		t.Fatal("missing balance query", got)
	}
	c.dispatch([]byte{35, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if f.State.Busy() || len(f.State.Cart) != 1 || len(sent()) != 1 {
		t.Fatal("zero points sent purchase")
	}
	f.Pay.Click()
	c.dispatch([]byte{35, 4, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	got := sent()
	if len(got) != 3 || !bytes.Equal(got[2], []byte{75, 1, 1, 154, 152, 5, 2, 1, 0}) {
		t.Fatal("purchase contract", got)
	}
	f.Pay.Click()
	if len(sent()) != 3 {
		t.Fatal("duplicate pending checkout")
	}
	*now = now.Add(20 * time.Second)
	f.Update(&seui.Input{})
	if !f.State.WaitingReceipt || f.Pay.Enabled {
		t.Fatal("timeout enabled unsafe purchase retry")
	}
	c.dispatch([]byte{75, 4, 154, 152, 5, 2, 1, 0, 1})
	if f.State.Busy() || len(f.State.Cart) != 0 {
		t.Fatal("late confirmed receipt not applied")
	}
	if !c.MallKey(escapeKey) || f.Visible {
		t.Fatal("escape")
	}
}

func TestMallTabsMalformedCatalogAndLifecycle(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.dispatch(mallCatalogFixture())
	c.dispatch([]byte{75, 7, 1})
	c.Mall.Show()
	c.Mall.Tabs[3].Click()
	if len(c.Mall.Entries()) != 1 || c.Mall.Page != 1 {
		t.Fatal("category filtering")
	}
	c.Mall.Add[0].Click()
	c.Mall.Return.Click()
	c.Mall.Bonus.Click()
	if !c.Mall.State.Bonus || len(c.Mall.State.Cart) != 0 {
		t.Fatal("currency mixed carts")
	}
	before := len(c.Mall.State.Catalog[0])
	c.dispatch([]byte{75, 1, 1, 0})
	if len(c.Mall.State.Catalog[0]) != before {
		t.Fatal("malformed catalog replaced state")
	}
	c.Mall.Hide()
	c.held = true
	c.toggleMall()
	if c.Mall.Visible {
		t.Fatal("event lock ignored")
	}
	c.held = false
	c.toggleMall()
	if !c.Mall.Visible {
		t.Fatal("toolbar did not open")
	}
	c.disconnected()
	if c.Mall.Visible || len(c.Mall.State.Catalog[0]) != 0 {
		t.Fatal("disconnect retained mall")
	}
}

func TestMallNativePresentation(t *testing.T) {
	if os.Getenv("MALL_SNAPSHOT") == "" {
		t.Skip("optional native visual check")
	}
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.dispatch(mallCatalogFixture())
	c.dispatch([]byte{75, 7, 1})
	c.Stats.Level = 14
	c.Stats.Element = 3
	c.Stats.HP = 450
	c.Stats.MaxHP = 450
	c.Stats.SP = 231
	c.Stats.MaxSP = 231
	c.Stats.Combat = [5]uint16{86, 42, 24, 46, 24}
	c.Stats.STR = 29
	c.Stats.CON = 8
	c.Stats.INT = 2
	c.Stats.WIS = 8
	c.Stats.AGI = 1
	c.Mall.Show()
	for _, name := range []string{"Form_Shopping_1", "Form_ShoppingCar_1", "ShopTab_1", "Btn_InCar"} {
		if c.Pics.Find(name) < 0 {
			t.Fatal("missing native asset", name)
		}
	}
	c.Frame()
	savePNG(t, filepath.Join(os.Getenv("MALL_SNAPSHOT"), "catalog.png"), c)
	c.Mall.AddRow(0)
	c.Frame()
	savePNG(t, filepath.Join(os.Getenv("MALL_SNAPSHOT"), "cart.png"), c)
}

func TestMallHotViewUsesNativeBadgesAndWireCategories(t *testing.T) {
	c, _, _ := enteredClient(t)
	for _, id := range []uint16{65000, 65001, 65002, 65003, 65004} {
		c.items[id] = assets.NativeItem{}
	}
	f := c.Mall
	f.State.Catalog[0] = []mall.Entry{{ID: 65000, Category: 1, Badge: 1}, {ID: 65001, Category: 2, Badge: 2}, {ID: 65002, Category: 3, Badge: 4}, {ID: 65003, Category: 4, Badge: 0}, {ID: 65004, Category: 5, Badge: 3}}
	rows := f.Entries()
	if len(rows) != 4 || rows[0].ID != 65002 || rows[1].ID != 65001 || rows[2].ID != 65000 || rows[3].ID != 65004 {
		t.Fatal("native Hot grouping", rows)
	}
	f.Category = mall.TabGrocery
	if rows = f.Entries(); len(rows) != 2 {
		t.Fatal("native 3/4 category merge", rows)
	}
}

func TestMallAuthoredHotContainsOnlyNewContainers(t *testing.T) {
	c, _, _ := enteredClient(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "internal", "assetsql", "mall_defaults.json"))
	if err != nil {
		t.Fatal(err)
	}
	var authored []assets.MallItem
	if err := json.Unmarshal(raw, &authored); err != nil {
		t.Fatal(err)
	}
	f := c.Mall
	for _, e := range authored {
		if e.Bonus != 0 || e.ID == 56276 {
			continue
		} // Incomplete pool is not advertised by Server.
		f.State.Catalog[0] = append(f.State.Catalog[0], mall.Entry{ID: e.ID, Count: e.Count, Cost: uint16(e.Cost), Badge: byte(e.Badge), Category: byte(e.CategoryID), Order: uint16(e.Order)})
	}
	f.Show()
	featured := map[uint16]bool{}
	for _, e := range f.Entries() {
		featured[e.ID] = true
	}
	if len(featured) != 20 || featured[57819] || !featured[35450] || !featured[35451] || !featured[34174] {
		t.Fatal("Hot membership", featured)
	}
	if f.Tabs[0].Image != c.Pics.Find("ShopTab_1") || f.Pages() != 2 {
		t.Fatal("Hot tab changed")
	}
	f.Tabs[3].Click()
	ordinary := map[uint16]bool{}
	for _, e := range f.Entries() {
		ordinary[e.ID] = true
	}
	if !ordinary[57819] {
		t.Fatal("old item removed from category")
	}
	for id := range featured {
		if !ordinary[id] {
			t.Fatal("featured container missing from category", id)
		}
	}
}

func TestMallPromotionalBadgesDoNotRender(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch(mallCatalogFixture())
	f := c.Mall
	f.Show()
	f.Tabs[3].Click()
	f.State.Catalog[0][1].Badge = 0
	c.Frame()
	before := append([]byte(nil), c.Screen.RGBA().Pix...)
	for _, badge := range []byte{1, 2, 3, 4} {
		f.State.Catalog[0][1].Badge = badge
		c.Frame()
		if !bytes.Equal(before, c.Screen.RGBA().Pix) {
			t.Fatal("promotional badge drawn", badge)
		}
	}
}

func TestMallClosedPreflightDoesNotSubmitOnLateBalance(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.dispatch(mallCatalogFixture())
	c.dispatch([]byte{75, 7, 1})
	c.Mall.Show()
	c.Mall.AddRow(0)
	c.Mall.CheckOut()
	if !c.Mall.State.WaitingBalance {
		t.Fatal("missing preflight")
	}
	c.Mall.Hide()
	c.dispatch([]byte{35, 4, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if len(sent()) != 1 || c.Mall.State.Busy() {
		t.Fatal("closed preflight submitted purchase")
	}
}

func TestMallStackableGroceryQuantityEditor(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.items[65000] = assets.NativeItem{Definition: game.ItemDefinition{ID: 65000, Type: 23}}
	f := c.Mall
	f.State.Enabled = true
	f.State.Catalog[0] = []mall.Entry{{ID: 65000, Count: 1, Cost: 10, Discount: 100, Category: 3, Badge: 1, Order: 1}}
	f.Show()
	f.AddRow(0)
	if f.Quantities[0].ReadOnly {
		t.Fatal("grocery quantity locked")
	}
	f.Quantities[0].KeyDown(seui.VKHome, 0)
	f.Quantities[0].KeyDown(seui.VKDelete, 0)
	f.Quantities[0].Char('9')
	f.Quantities[0].Char('9')
	f.Quantities[0].OnBlur()
	if f.State.Cart[0].Quantity != 50 || string(f.Quantities[0].Text) != "50" {
		t.Fatal("quantity bound", f.State.Cart, f.Quantities[0].Text)
	}
}

func TestMallPurchaseWriteFailureRemainsLocked(t *testing.T) {
	c, _, _ := enteredClient(t)
	f := c.Mall
	c.dispatch(mallCatalogFixture())
	c.dispatch([]byte{75, 7, 1})
	f.Show()
	f.AddRow(0)
	count := 0
	f.Send = func([]byte) error { count++; return errors.New("unconfirmed write") }
	f.State.WaitingBalance = true
	f.Balance([]byte{35, 4, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.CheckOut()
	if !f.State.WaitingReceipt || count != 1 {
		t.Fatal("failed write enabled duplicate purchase", count)
	}
}
