package app

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func lastNotice(c *Client) string {
	if len(c.Notices.items) == 0 {
		return ""
	}
	return string(c.Notices.items[len(c.Notices.items)-1].text)
}

// TestTradeRequest: the toolbar's Trade shows the Business menu; Trade
// Items needs level 25 and 25/44 2, then arms mode 9 with cursor 2; a
// click on a player of level 25 or more sends 25/1.
func TestTradeRequest(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	capture := wire(t, c)
	c.Stats.Level = 30
	low := &world.Peer{Player: world.Player{ID: 20004, Name: []byte("Low"), X: c.World.Player.X + 50, Y: c.World.Player.Y}, Level: 24}
	reborn := &world.Peer{Player: world.Player{ID: 20005, Name: []byte("Old"), X: c.World.Player.X - 80, Y: c.World.Player.Y}, Level: 1, Rebirth: 1}
	c.World.Peers = map[uint32]*world.Peer{low.ID: low, reborn.ID: reborn}
	c.FuncButtons.Buttons[tradeButton].Click()
	if !c.Business.Visible {
		t.Fatal("Business menu not shown")
	}
	c.Business.TradeItems.Click()
	if c.tradeTarget || lastNotice(c) != "Not enough online time to use!" {
		t.Fatalf("armed without 25/44 2: %q", lastNotice(c))
	}
	c.dispatch([]byte{protocol.CommandTrade, protocol.TradeOnlineTime, protocol.TradeOnlineTimeEnough})
	c.Stats.Level = 24
	c.Business.TradeItems.Click()
	if c.tradeTarget || lastNotice(c) != "Requires player LV25!" {
		t.Fatal("armed below level 25")
	}
	c.Stats.Level = 30
	c.Business.TradeItems.Click()
	c.Input.Hovered = nil
	c.updateCursor(c.Now())
	if !c.tradeTarget || c.Business.Visible || c.Cursors.Current != cursor.ShapePoint {
		t.Fatal("Trade Items did not arm mode 9 with cursor 2")
	}
	cx, cy := c.World.Camera()
	c.GroundClick(low.X-cx, low.Y-cy-20)
	if p := capture(); c.tradeTarget || len(p) != 0 || lastNotice(c) != "Player is below LV25!" {
		t.Fatalf("low-level target: %x %q", p, lastNotice(c))
	}
	c.FuncButtons.Buttons[tradeButton].Click()
	c.Business.TradeItems.Click()
	c.GroundClick(reborn.X-cx, reborn.Y-cy-20)
	want := protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(reborn.ID)
	if p := capture(); c.tradeTarget || len(p) != 1 || !bytes.Equal(p[0], want) {
		t.Fatalf("request %x", p)
	}
	c.World.Player.Map = mapNoTradeA
	c.FuncButtons.Buttons[tradeButton].Click()
	c.Business.TradeItems.Click()
	if c.tradeTarget || lastNotice(c) != "Can't trade here" {
		t.Fatal("armed on a no-trade map")
	}
}

// TestTradeForm: 25/1 opens the form; bag clicks build the offer by item
// size; Confirm sends the gold and slots; the partner's 25/3 fills its
// grid; Trade sends 25/3 1 and 25/2 closes the form with 25/3 2.
func TestTradeForm(t *testing.T) {
	c, _, _ := enteredClient(t)
	capture := wire(t, c)
	c.World.Peers = map[uint32]*world.Peer{20006: {Player: world.Player{ID: 20006, Name: []byte("Pal")}}}
	c.InventoryState.Items[32176] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32176, CellWidth: 1, CellHeight: 1}}
	c.InventoryState.Items[40001] = assets.NativeItem{Definition: game.ItemDefinition{ID: 40001, CellWidth: 2, CellHeight: 2}}
	c.InventoryState.Bag[0] = game.Item{ID: 32176, Count: 10}
	c.InventoryState.Bag[1] = game.Item{ID: 32176, Count: 1, Locked: true}
	c.InventoryState.Bag[3] = game.Item{ID: 40001, Count: 1}
	c.Stats.Gold = 40
	f := c.Trade
	c.dispatch(protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(20006))
	if !f.Visible || f.Partner != 20006 || string(f.PartnerName.Text) != "Pal" {
		t.Fatal("25/1 did not open the form for the player in view")
	}
	click := func(cell interface {
		LeftDown(byte, int, int)
		LeftUp(byte, int, int)
	}, x, y int) {
		cell.LeftDown(0, x, y)
		cell.LeftUp(0, x, y)
	}
	at := f.Bag[0].Abs()
	click(f.Bag[0], at.X+1, at.Y+1)
	at = f.Bag[1].Abs()
	click(f.Bag[1], at.X+1, at.Y+1)
	if lastNotice(c) != "Item locked, can't use@" {
		t.Fatal("locked item not refused")
	}
	at = f.Bag[3].Abs()
	click(f.Bag[3], at.X+1, at.Y+1)
	if f.Mine[0].Item().ID != 32176 || f.Mine[1].Item().ID != 40001 || f.Mine[2].Item().ID != 0 || f.Mine[6].Item().ID != 0 {
		t.Fatal("offer not placed by size")
	}
	if f.Offer(1) {
		t.Fatal("an offered stack was offered twice")
	}
	f.Withdraw(8) // a cell the 2 × 2 item covers
	if f.Mine[1].Item().ID != 0 || f.Offer(4) != true || f.Mine[1].Item().ID != 40001 {
		t.Fatal("withdraw by covered cell")
	}
	f.Gold.SetText([]byte("50"))
	f.Confirm.Click()
	if p := capture(); len(p) != 0 || lastNotice(c) != "Not enough Gold" {
		t.Fatalf("confirm above own gold %x", p)
	}
	c.Stats.Gold = 100
	f.Confirm.Click()
	offer := []byte{protocol.CommandTrade, protocol.TradeOffer, 50, 0, 0, 0, 1, 4}
	if p := capture(); len(p) != 1 || !bytes.Equal(p[0], offer) || !f.Confirmed || f.Confirm.Visible {
		t.Fatalf("confirm %x", p)
	}
	if f.Offer(5) {
		t.Fatal("a locked offer changed")
	}
	// The partner's 25/3, built as the server writes it (golden bytes in
	// internal/server/trade_test.go).
	item := game.Item{ID: 32176, Count: 3, Damage: 2}
	item.Metadata[0], item.Metadata[6], item.Metadata[20] = 1, 7, 5
	m := item.Metadata
	partner := protocol.Builder{protocol.CommandTrade, protocol.TradePartnerOffer}.U32(12345).U16(item.ID).U8(item.Count).U8(item.Damage).Bytes(m[2:8]).Bytes(m[0:2]).Bytes(m[8:])
	c.dispatch(partner)
	if f.Theirs[0].Item() != item || string(f.PartnerGold.Text) != "12345" || f.PartnerGold.Color != 0x001f || !f.PartnerOffered {
		t.Fatalf("partner offer %+v %q", f.Theirs[0].Item(), f.PartnerGold.Text)
	}
	f.Paint()
	if !f.Trade.Visible {
		t.Fatal("Trade did not show")
	}
	f.Trade.Click()
	if p := capture(); len(p) != 2 || !bytes.Equal(p[1], []byte{protocol.CommandTrade, protocol.TradeDecision, protocol.TradeDecisionAccept}) {
		t.Fatalf("trade %x", p)
	}
	c.dispatch([]byte{protocol.CommandTrade, protocol.TradeEnded, protocol.TradeResultCompleted})
	if p := capture(); f.Visible || len(p) != 3 || !bytes.Equal(p[2], []byte{protocol.CommandTrade, protocol.TradeDecision, protocol.TradeDecisionCancel}) || lastNotice(c) != "Trade complete" {
		t.Fatalf("25/2 did not close: %x %q", p, lastNotice(c))
	}
}

// TestTradeFits: Trade refuses while the partner's items cannot fit the
// bag (FUN_00367aac).
func TestTradeFits(t *testing.T) {
	c, _, _ := enteredClient(t)
	capture := wire(t, c)
	c.World.Peers = map[uint32]*world.Peer{20007: {Player: world.Player{ID: 20007, Name: []byte("Sam")}}}
	c.InventoryState.Items[32176] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32176, CellWidth: 1, CellHeight: 1}}
	for i := range c.InventoryState.Bag {
		c.InventoryState.Bag[i] = game.Item{ID: 32176, Count: 1}
	}
	c.dispatch(protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(20007))
	var m [game.ItemMetadataBytes]byte
	c.dispatch(protocol.Builder{protocol.CommandTrade, protocol.TradePartnerOffer}.U32(0).U16(32176).U8(1).U8(0).Bytes(m[:]))
	c.Trade.Trade.Click()
	if p := capture(); len(p) != 0 || lastNotice(c) != "Inventory full, can't trade" {
		t.Fatalf("full bag traded: %x", p)
	}
	c.InventoryState.Bag[49] = game.Item{}
	c.Trade.Trade.Click()
	if p := capture(); len(p) != 1 {
		t.Fatalf("room left but not traded: %x", p)
	}
}

// TestTradeSnapshot (opt-in) renders the Business menu and a trade under
// way; no capture of either form exists.
func TestTradeSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, _, _ := enteredClient(t)
	c.Stats.Gold = 123456
	c.Business.Show()
	save(t, c, filepath.Join(dir, "business.png"))
	c.Business.Hide()
	var ids []uint16
	for id, n := range c.InventoryState.Items {
		if n.Icon != 0 && n.Definition.CellWidth <= 1 && n.Definition.CellHeight <= 1 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) < 6 {
		t.Skip("no item catalog")
	}
	for i := 0; i < 6; i++ {
		id := ids[i*40%len(ids)]
		c.InventoryState.Bag[i*3] = game.Item{ID: id, Count: byte(i + 1)}
		c.loadPictures(strconv.Itoa(int(c.items[id].Icon)))
	}
	c.World.Peers = map[uint32]*world.Peer{20006: {Player: world.Player{ID: 20006, Name: []byte("Pal")}}}
	c.dispatch(protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(20006))
	c.Trade.Offer(1)
	c.Trade.Offer(4)
	c.Trade.Gold.SetText([]byte("500"))
	var m [game.ItemMetadataBytes]byte
	c.dispatch(protocol.Builder{protocol.CommandTrade, protocol.TradePartnerOffer}.U32(25000).U16(ids[7]).U8(3).U8(0).Bytes(m[:]).U16(ids[9]).U8(1).U8(0).Bytes(m[:]))
	save(t, c, filepath.Join(dir, "trade.png"))
}
