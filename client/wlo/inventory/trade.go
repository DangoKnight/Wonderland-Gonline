package inventory

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// TradeForm is TRe_TradeItemForm (constructor FUN_003658a8, paint
// FUN_003676cc, PTR_DAT_004ca040): Form_TradeItem_1, a fixed form centred
// in the main area, with the bag on the right (the item-grid base
// FUN_0035c238, cells from (0xce, 0x44)), the partner's offer above
// "money earned" and the player's own above "money paid". Both offers are
// 5 × 4 grids where an item covers its cell width × height
// (FUN_00366ee0). A click on a bag item offers its whole stack
// (0x366a94 → FUN_00366d9c), a click on an offered item takes it back
// (0x3671a8 → FUN_003671e4). Confirm (FUN_003680f4) locks the offer and
// sends 25/2; once the partner's 25/3 has arrived the Trade button
// (0x368000) shows and sends 25/3 1. Closing (FUN_00367dfc) sends 25/3 2.
type TradeForm struct {
	seui.FixedForm
	Inventory *Form
	Bag       [game.BagSize]*tradeCell // the base's ItemImage cells
	Theirs    [tradeCells]*tradeCell   // +0x1c4 "OtherSafeTradeItem"
	Mine      [tradeCells]*tradeCell   // +0x3e0 "MySafeTradeItem"

	PartnerName, PartnerGold, Gold *seui.Editor      // +0x474, +0x478, +0x47c
	CloseCorner, Cancel            *seui.FixedButton // +0x43c, +0x438
	Trade, Confirm                 *seui.FixedButton // +0x440, +0x448

	Partner        uint32 // +0x484
	Confirmed      bool   // +0x480: the offer is locked
	PartnerOffered bool   // +0x48d: the partner's 25/3 has arrived
	Decision       byte   // +0x48c: what 25/3 last sent

	offer        [tradeCells + 1]byte      // +0x214: the bag slot at each anchor
	mine, theirs [tradeCells + 1]int       // +0x2bc, +0x268: the anchor covering each cell
	partnerItems [tradeCells + 1]game.Item // the partner's items by anchor
	pressed      byte                      // +0x460: the bag cell pressed (+0x48e)

	Send func([]byte)
	// Notice shows a timed notice on the notice board.
	Notice func(string, time.Duration)
	// Busy is a running minigame (PTR_DAT_004c9994 +4).
	Busy func() bool
	// Gold is the player's gold (+0x3b44).
	PlayerGold func() uint32
}

const (
	tradeNativeClass = "TRe_TradeItemForm"
	tradeWidth       = 0x191
	tradeHeight      = 0x1e1
	tradeColumns     = 5
	tradeRows        = 4
	tradeCells       = tradeColumns * tradeRows
	tradeBagX        = 0xce
	tradeBagY        = 0x44
	tradeOffersX     = 0x18
	tradeTheirsY     = 0x45
	tradeMineY       = 0xfa
	tradeInk         = 0x0841
	// tradeGoldDigits is the gold field's length (FUN_00468e8c 9).
	tradeGoldDigits = 9
	// tradeGoldLimit is the gold field's check (0x367a38).
	tradeGoldLimit = 0x7fffffff
	// tradeOfferRecord is a 25/3 item: ID, count, damage and the bag
	// record's 26 metadata bytes (FUN_003c704c).
	tradeOfferRecord = 4 + game.ItemMetadataBytes
	// The original divides by 29 for the count (FUN_003c704c), which
	// gives the same answer for the 20 items a trade can hold.
	tradeOfferDivisor = 29

	// tradeOverlayPicture ends "<w>x<h>Grid_2", the startup table at
	// PTR_DAT_004c99d0 +0x120 that ItemImage draws under an item in a trade.
	tradeOverlayPicture = "Grid_2"

	tradeNoticeShort = 1200 * time.Millisecond
	tradeNotice      = 2000 * time.Millisecond
	tradeNoticeLong  = 3000 * time.Millisecond
	tradeNoticeBrief = 1000 * time.Millisecond

	noticeItemLocked      = "ui.inventory.item_locked_can_t_use_099d3a94"
	noticeTooMuchGold     = "ui.inventory.can_t_give_that_much_at_once_0193a7c9"
	noticeNotEnoughGold   = "ui.inventory.not_enough_gold_c1d5d70f"
	noticeFeatureDisabled = "ui.inventory.feature_disabled_055d9a02"
	noticeInventoryFull   = "ui.inventory.inventory_full_can_t_trade_74e40a51"
	noticeTooManyItems    = "ui.inventory.can_t_trade_that_much_items_56ca77dc"
)

// Bag items the click path refuses without a notice (0x366a94): the
// record's non-tradeable flag, and these instance fields (+0x32 a word,
// +0x34, and +0x35 = 1 except for item 34515), named after their effect.
const (
	tradeRefusedWordOffset = 20 // instance +0x32
	tradeRefusedByteOffset = 22 // instance +0x34
	tradeRefusedFlagOffset = 23 // instance +0x35
	tradeRefusedFlag       = 1
	tradeRefusedFlagExempt = 34515 // 0x86d3
)

type tradeGrid byte

const (
	tradeBag tradeGrid = iota
	tradeTheirs
	tradeMine
)

type tradeCell struct {
	seui.Component
	form  *TradeForm
	grid  tradeGrid
	index int // 1-based: the bag slot or the grid cell
}

// NewTradeForm is FUN_003658a8.
func NewTradeForm(env *seui.Env, inventory *Form) *TradeForm {
	f := &TradeForm{Inventory: inventory}
	f.InitFixedForm(f, env)
	f.Name = tradeNativeClass
	f.Init("Form_TradeItem_1", (seui.MainAreaW-tradeWidth)/2, 0, 0, 0, 0, false, tradeHeight, tradeWidth, (seui.MainAreaH-tradeHeight)/2)
	for i := range f.Bag {
		f.Bag[i] = f.cell(tradeBag, i+1, tradeBagX, tradeBagY)
	}
	for i := range f.Theirs {
		f.Theirs[i] = f.cell(tradeTheirs, i+1, tradeOffersX, tradeTheirsY)
	}
	for i := range f.Mine {
		f.Mine[i] = f.cell(tradeMine, i+1, tradeOffersX, tradeMineY)
	}
	editor := func(left, top, w, h int) *seui.Editor {
		e := seui.NewEditor(env, f)
		e.Init("", left, 0, 0, 0, 0, false, h, w, top)
		e.SetColor(tradeInk)
		return e
	}
	f.PartnerName = editor(0x5c, 0x2b, 0xa0, 0x14)
	f.PartnerName.ReadOnly = true
	f.PartnerGold = editor(0x5c, 0xd5, 100, 0x18)
	f.PartnerGold.SetAlign(2)
	f.PartnerGold.ReadOnly = true
	f.PartnerGold.SetFilter(1)
	f.PartnerGold.SetText([]byte("0"))
	f.Gold = editor(0x5c, 0x186, 100, 0x18)
	f.Gold.SetAlign(2)
	f.Gold.SetFilter(1)
	f.Gold.SetText([]byte("0"))
	f.Gold.OnChange = f.goldChanged
	f.Gold.SetMaxLen(tradeGoldDigits)
	button := func(picture string, left, top, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, f)
		b.Init(picture, left, h, w, 0, 0, true, h, w, top)
		b.OnClick = click
		return b
	}
	icon := func(picture string, left, top int) {
		p := seui.NewPanel(env, f)
		p.Init(picture, left, 0x14, 0x38, 0, 0, true, 0x14, 0x38, top)
		p.Hover = false
	}
	f.CloseCorner = button("Btn_Close_s_1", 0x160, 0x19, 0x12, 0x12, f.Close)
	f.Cancel = button("Btn_Cancel_1", 0xfa, 0x1bd, 0x38, 0x14, f.Close)
	f.Trade = button("Btn_Trade_2", 0xae, 0x1bd, 0x38, 0x14, f.accept)
	f.Trade.SetVisible(false)
	icon("icon_trade_1", 0xae, 0x1bd)
	f.Confirm = button("Btn_confirm_1", 100, 0x1bd, 0x38, 0x14, f.confirm)
	icon("icon_Confirm_1", 100, 0x1bd)
	return f
}

func (f *TradeForm) cell(grid tradeGrid, index, x, y int) *tradeCell {
	c := &tradeCell{form: f, grid: grid, index: index}
	c.InitComponent(c, f.Env, f)
	col, row := (index-1)%tradeColumns, (index-1)/tradeColumns
	c.Init("", x+col*cellStep, 0, 0, 0, 0, false, cellSize, cellSize, y+row*cellStep)
	c.DownSound = 0xff
	return c
}

func (f *TradeForm) notice(s string, d time.Duration) {
	if f.Notice != nil {
		f.Notice(s, d)
	}
}
func (f *TradeForm) send(p []byte) {
	if f.Send != nil {
		f.Send(p)
	}
}
func (f *TradeForm) busy() bool { return f.Busy != nil && f.Busy() }

// Open is FUN_00367d80 with FUN_00366320: the form shows for a partner.
func (f *TradeForm) Open(partner uint32, name []byte) {
	f.reset()
	f.Partner = partner
	f.PartnerName.SetText(name)
	f.Gold.SetColor(tradeInk)
	f.pressed = 0
	f.Show()
}

// Reset hides the form without telling the server.
func (f *TradeForm) Reset() {
	f.reset()
	f.pressed = 0
	f.Hide()
}

func (f *TradeForm) reset() {
	f.Confirmed, f.PartnerOffered = false, false
	f.offer = [tradeCells + 1]byte{}
	f.mine, f.theirs = [tradeCells + 1]int{}, [tradeCells + 1]int{}
	f.partnerItems = [tradeCells + 1]game.Item{}
	f.PartnerName.SetText(nil)
	f.PartnerGold.SetText([]byte("0"))
	f.Gold.SetText([]byte("0"))
	f.Confirm.SetVisible(true)
	f.Trade.SetVisible(false)
	f.Partner = 0
}

// Close is FUN_00367dfc: everything resets, 25/3 2 goes out and the form
// hides. The original sends it whether the form was open or not.
func (f *TradeForm) Close() {
	f.Decision = protocol.TradeDecisionCancel
	f.send([]byte{protocol.CommandTrade, protocol.TradeDecision, f.Decision})
	f.reset()
	f.pressed = 0
	f.Hide()
}

// size is an item's cells (item table +0x19a, +0x19b).
func (f *TradeForm) size(id uint16) (int, int) {
	d := f.Inventory.State.Items[id].Definition
	return max(1, int(d.CellWidth)), max(1, int(d.CellHeight))
}

// place is FUN_00366ee0's search: the first anchor where the item's cells
// are inside the 5 × rows grid and free, marked with that anchor.
func place(grid []int, rows, w, h int) int {
	for pos := 1; pos < len(grid); pos++ {
		col, row := (pos-1)%tradeColumns, (pos-1)/tradeColumns
		if col+w > tradeColumns || row+h > rows || !free(grid, col, row, w, h) {
			continue
		}
		for y := row; y < row+h; y++ {
			for x := col; x < col+w; x++ {
				grid[y*tradeColumns+x+1] = pos
			}
		}
		return pos
	}
	return 0
}
func free(grid []int, col, row, w, h int) bool {
	for y := row; y < row+h; y++ {
		for x := col; x < col+w; x++ {
			if grid[y*tradeColumns+x+1] != 0 {
				return false
			}
		}
	}
	return true
}

// offered reports whether a bag slot is in the offer.
func (f *TradeForm) offered(slot byte) bool {
	for _, s := range f.offer[1:] {
		if s == slot {
			return true
		}
	}
	return false
}

// tradeable is 0x366a94's silent checks.
func (f *TradeForm) tradeable(it game.Item) bool {
	n := f.Inventory.State.Items[it.ID]
	if binary.LittleEndian.Uint16(n.Record[itemTradeFlagsOffset:])&itemNonTradeableFlag != 0 {
		return false
	}
	m := it.Metadata
	if binary.LittleEndian.Uint16(m[tradeRefusedWordOffset:]) != 0 || m[tradeRefusedByteOffset] != 0 {
		return false
	}
	return it.ID == tradeRefusedFlagExempt || m[tradeRefusedFlagOffset] != tradeRefusedFlag
}

// Offer is 0x366a94 → FUN_00366d9c: a bag item joins the offer.
func (f *TradeForm) Offer(slot byte) bool {
	if f.Confirmed || slot < 1 || int(slot) > game.BagSize || f.offered(slot) {
		return false
	}
	it := f.Inventory.State.Bag[slot-1]
	if it.Empty() || it.Locked || !f.tradeable(it) {
		return false
	}
	w, h := f.size(it.ID)
	if w > tradeColumns || h > tradeRows {
		return false
	}
	pos := place(f.mine[:], tradeRows, w, h)
	if pos == 0 {
		return false
	}
	f.offer[pos] = slot
	return true
}

// Withdraw is FUN_003671e4: the offered item under a cell goes back.
func (f *TradeForm) Withdraw(cell int) {
	if f.Confirmed || cell < 1 || cell > tradeCells {
		return
	}
	pos := f.mine[cell]
	if pos == 0 {
		return
	}
	for i := range f.mine {
		if f.mine[i] == pos {
			f.mine[i] = 0
		}
	}
	f.offer[pos] = 0
}

// refresh drops offered items whose bag slot changed (the original's
// cells follow the bag form's, +0x13e).
func (f *TradeForm) refresh() {
	if f.Confirmed {
		return
	}
	for pos, slot := range f.offer {
		if slot != 0 && f.Inventory.State.Bag[slot-1].Empty() {
			for i := range f.mine {
				if f.mine[i] == pos {
					f.mine[i] = 0
				}
			}
			f.offer[pos] = 0
		}
	}
}

// goldChanged is 0x367a38.
func (f *TradeForm) goldChanged() {
	if v, err := strconv.ParseUint(string(f.Gold.Text), 10, 64); err == nil && v > tradeGoldLimit {
		f.notice(localization.UI(noticeTooMuchGold), tradeNoticeShort)
	}
}

// confirm is FUN_003680f4.
func (f *TradeForm) confirm() {
	if len(f.Gold.Text) == 0 {
		f.Gold.SetText([]byte("0"))
	}
	gold, _ := strconv.ParseUint(string(f.Gold.Text), 10, 64)
	if f.PlayerGold == nil || gold > uint64(f.PlayerGold()) {
		f.notice(localization.UI(noticeNotEnoughGold), tradeNoticeShort)
		return
	}
	p := protocol.Builder{protocol.CommandTrade, protocol.TradeOffer}.U32(uint32(gold))
	for _, slot := range f.offer[1:] {
		if slot != 0 {
			p = append(p, slot)
		}
	}
	f.send(p)
	f.Confirmed = true
	f.Confirm.SetVisible(false)
}

// accept is 0x368000.
func (f *TradeForm) accept() {
	if f.busy() {
		f.notice(localization.UI(noticeFeatureDisabled), tradeNotice)
		return
	}
	if !f.fits() {
		f.notice(localization.UI(noticeInventoryFull), tradeNotice)
		return
	}
	f.Decision = protocol.TradeDecisionAccept
	f.send([]byte{protocol.CommandTrade, protocol.TradeDecision, f.Decision})
	f.Confirm.SetVisible(false)
	f.Trade.SetVisible(false)
}

// fits is FUN_00367aac: the partner's items, by size, fit around
// everything in the 5 × 10 bag (the player's offered items included).
func (f *TradeForm) fits() bool {
	bag := make([]int, game.BagSize+1)
	for i, it := range f.Inventory.State.Bag {
		if it.Empty() {
			continue
		}
		w, h := f.size(it.ID)
		col, row := i%tradeColumns, i/tradeColumns
		for y := row; y < min(row+h, BagRows); y++ {
			for x := col; x < min(col+w, tradeColumns); x++ {
				bag[y*tradeColumns+x+1] = i + 1
			}
		}
	}
	for pos := 1; pos <= tradeCells; pos++ {
		it := f.partnerItems[pos]
		if it.Empty() {
			continue
		}
		w, h := f.size(it.ID)
		if place(bag, BagRows, w, h) == 0 {
			return false
		}
	}
	return true
}

// PartnerOffer is 25/3 (FUN_003c704c): the partner's gold, coloured by
// its length, and items. An item too large for the grid ends the trade.
func (f *TradeForm) PartnerOffer(p []byte) bool {
	if len(p) < 6 {
		return false
	}
	gold := binary.LittleEndian.Uint32(p[2:])
	records := (len(p) - 6) / tradeOfferDivisor
	if records > tradeCells || 6+records*tradeOfferRecord > len(p) {
		return false
	}
	text := strconv.FormatUint(uint64(gold), 10)
	f.PartnerGold.SetText([]byte(text))
	f.PartnerGold.SetColor(tradeGoldInk(len(text)))
	f.theirs = [tradeCells + 1]int{}
	f.partnerItems = [tradeCells + 1]game.Item{}
	for i := 0; i < records; i++ {
		r := p[6+i*tradeOfferRecord:]
		it := game.Item{ID: binary.LittleEndian.Uint16(r), Count: r[2], Damage: r[3]}
		copy(it.Metadata[2:8], r[4:10])
		copy(it.Metadata[0:2], r[10:12])
		copy(it.Metadata[8:], r[12:tradeOfferRecord])
		w, h := f.size(it.ID)
		pos := 0
		if w <= tradeColumns && h <= tradeRows {
			pos = place(f.theirs[:], tradeRows, w, h)
		}
		if pos == 0 {
			f.notice(localization.UI(noticeTooManyItems), tradeNotice)
			f.Close()
			return true
		}
		f.partnerItems[pos] = it
	}
	f.PartnerOffered = true
	return true
}

// Ended is 25/2 (FUN_003c6bd0): the reason shows and the form closes;
// during a minigame nothing happens.
func (f *TradeForm) Ended(result byte) {
	if f.busy() {
		return
	}
	text, d := localization.UI("ui.inventory.interrupted_132d124d"), tradeNoticeBrief
	switch result {
	case protocol.TradeResultSystemBusy:
		text = localization.UI("ui.inventory.system_busy_trade_later_41217e0b")
	case protocol.TradeResultDisconnected:
		text = localization.UI("ui.inventory.player_disconnected_b4aeb5d8")
	case protocol.TradeResultBusy:
		text = localization.UI("ui.inventory.player_is_busy_d6a58476")
	case protocol.TradeResultCanceled:
		text = localization.UI("ui.inventory.canceled_13ca2ee2")
		if f.Visible {
			text = localization.UI("ui.inventory.trade_canceled_142b0185")
		}
	case protocol.TradeResultCompleted:
		text = localization.UI("ui.inventory.trade_complete_f9a432ee")
	case protocol.TradeResultDifferentMaps:
		text = localization.UI("ui.inventory.players_on_diff_maps_f0465d22")
	case protocol.TradeResultNotEnoughGold:
		text = localization.UI("ui.inventory.not_enough_gold_c1d5d70f")
	case protocol.TradeResultInventoryFull:
		text = localization.UI("ui.inventory.player_s_inventory_full_c747250a")
	case protocol.TradeResultUnavailable:
		text = localization.UI("ui.inventory.can_t_trade_now_7200f201")
	case protocol.TradeResultPartnerCanceled:
		text = localization.UI("ui.inventory.other_player_canceled_trade_4c5c479a")
	case protocol.TradeResultStallInUse:
		text = localization.UI("ui.inventory.stall_is_in_use_13b099ff")
	case protocol.TradeResultAlreadyTrading:
		text = localization.UI("ui.inventory.can_t_have_few_trades_55fb7918")
	case protocol.TradeResultPetCrafting:
		text = localization.UI("ui.inventory.can_t_trade_pet_is_crafting_53664374")
	}
	if text != localization.UI("ui.inventory.interrupted_132d124d") {
		d = tradeNoticeLong
	}
	f.notice(text, d)
	f.Close()
}

// tradeGoldInk is FUN_003c704c's colour for the partner's gold.
func tradeGoldInk(digits int) uint16 {
	switch digits {
	case 4:
		return 0x0540
	case 5:
		return 0x001f
	case 6:
		return 0xf81e
	case 7:
		return 0xf800
	}
	return tradeInk
}

// Paint is FUN_003676cc: the Trade button shows while the partner's
// offer is in and the player's own is locked.
func (f *TradeForm) Paint() {
	f.refresh()
	if f.PartnerOffered && !f.Confirm.Visible {
		f.Trade.SetVisible(true)
	}
	f.FixedForm.Paint()
}

// Item is what a cell shows: a bag item, or an offered item at its anchor.
func (c *tradeCell) Item() game.Item {
	f := c.form
	switch c.grid {
	case tradeBag:
		return f.Inventory.State.Bag[c.index-1]
	case tradeMine:
		if f.mine[c.index] == c.index && f.offer[c.index] != 0 {
			return f.Inventory.State.Bag[f.offer[c.index]-1]
		}
	case tradeTheirs:
		if f.theirs[c.index] == c.index {
			return f.partnerItems[c.index]
		}
	}
	return game.Item{}
}

func (c *tradeCell) Paint() {
	it := c.Item()
	if it.Empty() {
		return
	}
	a := c.Abs()
	f := c.form
	if c.grid == tradeBag && f.offered(byte(c.index)) {
		w, h := f.size(it.ID)
		pics := f.Env.Pics
		pics.Draw(f.Env.Screen, pics.Find(strconv.Itoa(w)+"x"+strconv.Itoa(h)+tradeOverlayPicture), a.X, a.Y, true)
	}
	f.Inventory.drawItem(it, a.X, a.Y, true)
}

func (c *tradeCell) Hint() {
	if it := c.Item(); !it.Empty() && !c.Blocked() {
		c.form.Inventory.drawItemHint(&c.Component, it)
	}
}

// LeftDown is FUN_003673d8 for a bag cell: it remembers the press and
// warns about a locked item.
func (c *tradeCell) LeftDown(shift byte, x, y int) {
	c.Component.LeftDown(shift, x, y)
	f := c.form
	if c.grid != tradeBag || f.Confirmed || c.Blocked() {
		return
	}
	it := c.Item()
	if it.Empty() {
		return
	}
	if it.Locked {
		f.notice(localization.UI(noticeItemLocked), tradeNotice)
		return
	}
	f.pressed = byte(c.index)
	f.Env.UI.Input.Captured = c
}

// LeftUp is 0x366a94 for a bag cell and 0x3671a8 for an offer cell.
func (c *tradeCell) LeftUp(_ byte, x, y int) {
	f := c.form
	switch c.grid {
	case tradeBag:
		pressed := f.pressed
		f.pressed = 0
		if pressed == byte(c.index) && c.HitTest(x, y) {
			f.Offer(pressed)
		}
	case tradeMine:
		f.Withdraw(c.index)
	}
}
func (c *tradeCell) LeftUpOutside(_ byte, _, _ int) { c.form.pressed = 0 }
