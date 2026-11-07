package mall

import (
	"fmt"
	"image"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

// FUN_00244e20 constructs the catalog; FUN_00242e40 constructs the cart.
const (
	Width                 = 778
	Height                = 554
	CartWidth             = 548
	CartHeight            = 534
	catalogRowTop         = 36
	catalogRowHeight      = 40
	cartRowTop            = 49
	cartRowHeight         = 59
	previewFeetY          = 308 // Align the body above the HP gauge, as in the Legacy capture.
	mallInk               = 0x0841
	requestTimeout        = 15 * time.Second
	TabHot           byte = 1
	TabArmory        byte = 2
	TabWeaponry      byte = 3
	TabGrocery       byte = 4
	TabFurniture     byte = 5
	TabGames         byte = 6
	TabForging       byte = 7
)

type Form struct {
	IncompleteContents map[uint16]bool
	seui.FixedForm
	Games                                                                                                    [mallGamePageSize]*seui.FixedButton
	ForgeConfirm, ForgeCancel                                                                                *seui.FixedButton
	Bag                                                                                                      func() game.Inventory
	Confirm                                                                                                  func(string, func())
	ApplyForge                                                                                               func(byte, byte) bool
	ForgeUpgrades                                                                                            map[uint16]clientassets.MallForgeUpgrade
	ForgeSlot                                                                                                byte
	ForgeItem                                                                                                game.Item
	ForgePending                                                                                             bool
	ForgeAt                                                                                                  time.Time
	CompanionContents                                                                                        map[uint16]bool
	RewardName                                                                                               func(uint16) string
	RewardIcon                                                                                               func(uint16, image.Rectangle) bool
	FittingItems                                                                                             map[uint16]uint16
	Fitted                                                                                                   game.Equipment
	Equipment                                                                                                func() game.Equipment
	Contents                                                                                                 map[uint16][]clientassets.MallReward
	ContentsForm                                                                                             *ContentsForm
	State                                                                                                    State
	Items                                                                                                    map[uint16]assets.NativeItem
	Stats                                                                                                    *world.Stats
	PlayerName                                                                                               func() []byte
	Preview                                                                                                  func(int, int, int)
	LoadPictures                                                                                             func(...string)
	CanFit                                                                                                   func([]Row) bool
	Send                                                                                                     func([]byte) error
	Notice                                                                                                   func(string)
	Now                                                                                                      func() time.Time
	Category                                                                                                 byte
	Page, Direction                                                                                          int
	InCart                                                                                                   bool
	Tabs                                                                                                     [7]*seui.FixedButton
	Points, Bonus, Previous, Next, ViewCart, Checkout, Exit, Security, ResetPreview, RotateLeft, RotateRight *seui.FixedButton
	Add                                                                                                      [CatalogRows]*seui.FixedButton
	Delete                                                                                                   [CartRows]*seui.FixedButton
	Quantities                                                                                               [CartRows]*seui.Editor
	Return, Pay, CartExit                                                                                    *seui.FixedButton
	requestAt                                                                                                time.Time
	timeoutNotified                                                                                          bool
	status                                                                                                   *login.Status
}

func NewForm(env *seui.Env, items map[uint16]assets.NativeItem, stats *world.Stats) *Form {
	f := &Form{Items: items, Stats: stats, Category: TabHot, Page: 1, Direction: previewDefaultDirection, Now: time.Now}
	f.InitFixedForm(f, env)
	f.Name = "ItemMall"
	f.Draggable = false
	f.status = login.NewStatus(env, stats.Formula)
	f.Init("Form_Shopping_1", (800-Width)/2, Height, Width, 0, 0, true, Height, Width, (600-Height)/2)
	button := func(pic string, x, y, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, f)
		b.Init(pic, x, h, w, 0, 0, true, h, w, y)
		b.OnClick = click
		return b
	}
	f.Points = button("Btn_ShopPoint_1", 16, 38, 106, 21, func() { f.SelectCurrency(false) })
	f.Bonus = button("Btn_Bonus_1", 101, 38, 106, 21, func() { f.SelectCurrency(true) })
	for i := range f.Tabs {
		category := byte(i + 1)
		f.Tabs[i] = button(fmt.Sprintf("ShopTab_%d", category), 233+i*74, 12, 74, 22, func() { f.selectTab(category) })
	}
	for i := range f.Add {
		row := i
		f.Add[i] = button("Btn_InCar", 690, 47+i*catalogRowHeight, 71, 20, func() { f.AddRow(row) })
	}
	f.Previous = button("Btn_PrePage_1", 225, 523, 56, 20, func() { f.Page = max(1, f.Page-1); f.Refresh() })
	f.Next = button("Btn_NextPage_1", 359, 523, 56, 20, func() { f.Page = min(f.Pages(), f.Page+1); f.Refresh() })
	f.ViewCart = button("Btn_ViewCar", 501, 523, 71, 20, f.ShowCart)
	f.Checkout = button("Btn_PayOff", 591, 523, 71, 20, f.ShowCart)
	f.Exit = button("Btn_Exit", 681, 523, 71, 20, f.Hide)
	f.Security = button("Btn_ExchangeLockSet", 423, 523, 71, 20, func() { f.notice("ui.mall.security_unavailable") })
	f.Security.Enabled = false
	f.Security.BindLanguage("hint", func() { f.Security.SetHint([]byte(localization.UI("ui.mall.security_unavailable"))) })
	f.RotateLeft = button("Btn_ArrowL_8", 32, 294, 21, 21, func() { f.Direction = 8 + (f.Direction-8+7)%8 })
	f.RotateRight = button("Btn_ArrowR_8", 170, 294, 21, 21, func() { f.Direction = 8 + (f.Direction-8+1)%8 })
	f.ResetPreview = button("Btn_Reset", 83, 481, 56, 20, f.ResetFitting)
	for i := range f.Delete {
		row := i
		f.Delete[i] = button("Btn_DelItem", 445, 58+i*cartRowHeight, 56, 20, func() { f.State.Remove(row); f.Refresh() })
		e := seui.NewEditor(env, f)
		e.Init("", 400, 14, 28, 0, 0, false, 14, 28, 60+i*cartRowHeight)
		e.SetAlign(2)
		e.SetColor(mallInk)
		e.Filter = 1
		e.Limit = true
		e.MaxLen = 2
		e.OnChange = func() {
			if e.ReadOnly {
				return
			}
			if n, err := strconv.Atoi(string(e.Text)); err == nil {
				f.State.Quantity(row, n)
			}
		}
		e.OnBlur = func() { f.syncQuantity(row) }
		f.Quantities[i] = e
	}
	f.Return = button("Btn_Return", 80, 494, 71, 20, f.ShowCatalog)
	f.Pay = button("Btn_PayOffOK", 222, 494, 71, 20, f.CheckOut)
	f.CartExit = button("Btn_Exit", 368, 494, 71, 20, f.Hide)
	f.Contents = clientassets.DefaultMallContents()
	f.IncompleteContents = clientassets.MallIncompleteContents()
	f.CompanionContents = clientassets.MallCompanionContents()
	f.FittingItems = clientassets.DefaultMallFitting()
	f.ContentsForm = newContentsForm(f)
	f.initServices()
	f.Refresh()
	return f
}

func (f *Form) notice(key string) {
	if f.Notice != nil {
		f.Notice(localization.UI(key))
	}
}
func (f *Form) send(p []byte) bool {
	if len(p) == 0 {
		return false
	}
	if f.Send == nil {
		return false
	}
	if err := f.Send(p); err != nil {
		if f.Notice != nil {
			f.Notice(err.Error())
		}
		f.State.WaitingBalance = false
		// A write error cannot prove that a purchase was not received. Retain
		// its receipt lock until reconnect/confirmation, just like a timeout.
		f.timeoutNotified = true
		return false
	}
	f.requestAt = f.Now()
	f.timeoutNotified = false
	return true
}
func (f *Form) Entries() []Entry {
	var rows []Entry
	groups := []byte{f.Category}
	if f.Category == TabHot {
		groups = []byte{TabGrocery, TabArmory, TabWeaponry, TabFurniture}
	}
	for _, group := range groups {
		for _, e := range f.State.Catalog[f.State.Index()] {
			if _, known := f.Items[e.ID]; !known {
				continue
			}
			match := false
			switch group {
			case TabArmory:
				match = e.Category == protocol.MallCategoryArmory
			case TabWeaponry:
				match = e.Category == protocol.MallCategoryWeaponry
			case TabGrocery:
				match = e.Category == protocol.MallCategoryGrocery || e.Category == protocol.MallCategoryProps
			case TabFurniture:
				match = e.Category == protocol.MallCategoryFurniture
			case TabGames:
				match = e.Category == protocol.MallCategoryGames
			case TabForging:
				match = e.Category == protocol.MallCategoryForging
			}
			if f.Category == TabHot {
				limit := byte(3)
				if group == TabGrocery {
					limit = 4
				}
				match = match && e.Badge >= 1 && e.Badge <= limit
			}
			if match {
				rows = append(rows, e)
			}
		}
	}
	if f.Category == TabHot {
		rows = rows[:min(60, len(rows))]
	}
	return rows
}
func (f *Form) Pages() int {
	if f.Category == TabGames && !f.State.Bonus {
		return 2
	}
	return max(1, (len(f.Entries())+CatalogRows-1)/CatalogRows)
}
func (f *Form) SelectCurrency(bonus bool) {
	if f.State.Busy() || f.ForgePending || f.State.Bonus == bonus {
		return
	}
	f.State.Bonus = bonus
	if bonus && f.Category > TabFurniture {
		f.Category = TabHot
	}
	f.State.Cart = nil
	f.Page = 1
	f.ShowCatalog()
}
func (f *Form) AddRow(row int) {
	rows := f.Entries()
	index := (f.Page-1)*CatalogRows + row
	if f.State.Busy() || index < 0 || index >= len(rows) {
		return
	}
	if !f.State.Add(rows[index]) {
		f.notice("ui.mall.cart_full_or_duplicate")
		return
	}
	f.ShowCart()
}
func (f *Form) ShowCart() {
	f.InCart = true
	f.Left = (800 - CartWidth) / 2
	f.Top = 21
	f.Width = CartWidth
	f.Height = CartHeight
	f.Image = f.Env.Pics.Find("Form_ShoppingCar_1")
	f.Refresh()
	f.FixedForm.Show()
}
func (f *Form) ShowCatalog() {
	f.InCart = false
	f.Left = (800 - Width) / 2
	f.Top = (600 - Height) / 2
	f.Width = Width
	f.Height = Height
	f.Image = f.Env.Pics.Find("Form_Shopping_1")
	f.Refresh()
	f.FixedForm.Show()
}
func (f *Form) Show() { f.ShowCatalog() }
func (f *Form) Hide() {
	f.ContentsForm.Hide()
	// Closing can cancel a read-only preflight, never a submitted transaction.
	f.State.WaitingBalance = false
	f.FixedForm.Hide()
}
func (f *Form) Reset() {
	f.Hide()
	f.State.Reset()
	f.ForgePending = false
	f.ForgeSlot = 0
	f.ForgeItem = game.Item{}
	f.InCart = false
	f.Page = 1
	f.Category = TabHot
	f.ResetFitting()
	f.Refresh()
}
func (f *Form) syncQuantity(row int) {
	if row < len(f.State.Cart) {
		f.Quantities[row].SetText([]byte(strconv.Itoa(int(f.State.Cart[row].Quantity))))
	}
}
func (f *Form) CheckOut() {
	if !f.State.Enabled || f.State.Busy() {
		return
	}
	for i := range f.State.Cart {
		f.syncQuantity(i)
	}
	if len(f.State.Cart) == 0 {
		f.notice("ui.mall.empty_cart")
		return
	}
	if f.State.Bonus && f.State.Total() > uint64(f.State.Balances[1]) {
		f.notice("ui.mall.insufficient_bonus")
		return
	}
	if f.State.Bonus && f.CanFit != nil && !f.CanFit(f.State.Cart) {
		f.notice("ui.mall.inventory_full")
		return
	}
	f.send(f.State.RequestCheckout())
	f.Refresh()
}
func (f *Form) Balance(p []byte) bool {
	if !f.State.CheckoutBalance(p) {
		return false
	}
	if f.State.WaitingBalance {
		if f.State.Total() > uint64(f.State.Balances[0]) {
			f.State.WaitingBalance = false
			f.notice("ui.mall.insufficient_points")
		} else {
			if f.CanFit != nil && !f.CanFit(f.State.Cart) {
				f.State.WaitingBalance = false
				f.notice("ui.mall.inventory_full")
				f.Refresh()
				return true
			}
			f.send(f.State.Submit())
		}
	}
	f.Refresh()
	return true
}
func (f *Form) Receive(p []byte) bool {
	if err := f.State.Apply(p); err != nil {
		return false
	}
	if len(p) > 1 && (p[1] == protocol.MallPointsCatalog || p[1] == protocol.MallBonusCatalog) && f.LoadPictures != nil {
		index := 0
		if p[1] == protocol.MallBonusCatalog {
			index = 1
		}
		for _, e := range f.State.Catalog[index] {
			if icon := f.Items[e.ID].Icon; icon != 0 {
				f.LoadPictures(strconv.Itoa(int(icon)))
			}
		}
	}
	if len(p) == 9 && p[1] == protocol.MallCartReceipt && p[8] == protocol.MallPurchaseFailed {
		f.notice("ui.mall.purchase_failed")
	}
	f.Refresh()
	return true
}
func (f *Form) Refresh() {
	f.Page = max(1, min(f.Page, f.Pages()))
	for _, b := range []*seui.FixedButton{f.Points, f.Bonus, f.Previous, f.Next, f.ViewCart, f.Checkout, f.Exit, f.Security, f.ResetPreview, f.RotateLeft, f.RotateRight} {
		b.Visible = !f.InCart
	}
	f.Points.Toggled = !f.State.Bonus
	f.Points.Sticky = f.Points.Toggled
	f.Bonus.Toggled = f.State.Bonus
	f.Bonus.Sticky = f.Bonus.Toggled
	f.Points.State, f.Bonus.State = 0, 0
	if f.Points.Sticky {
		f.Points.State = 2
	} else {
		f.Bonus.State = 2
	}
	f.Points.Enabled = !f.State.Busy()
	f.Bonus.Enabled = !f.State.Busy()
	for i, b := range f.Tabs {
		b.Visible = !f.InCart
		b.Toggled = byte(i+1) == f.Category
		b.Sticky = b.Toggled
		b.State = 0
		if b.Sticky {
			b.State = 2
		}
	}
	rows := f.Entries()
	for i, b := range f.Add {
		b.Visible = !f.InCart && (f.Page-1)*CatalogRows+i < len(rows)
		b.Enabled = f.State.Enabled && !f.State.Busy()
	}
	f.Previous.Enabled = f.Page > 1
	f.Next.Enabled = f.Page < f.Pages()
	for i, b := range f.Delete {
		visible := f.InCart && i < len(f.State.Cart)
		b.Visible = visible
		b.Enabled = !f.State.Busy()
		f.Quantities[i].Visible = visible
		f.Quantities[i].ReadOnly = f.State.Busy()
		if visible {
			// Gonline permits quantity editing for every cart item. Freeze only
			// while a checkout is outstanding, preserving its exact receipt keys.
			f.syncQuantity(i)
		}
	}
	for _, b := range []*seui.FixedButton{f.Return, f.Pay, f.CartExit} {
		b.Visible = f.InCart
	}
	f.Pay.Enabled = f.State.Enabled && !f.State.Busy() && len(f.State.Cart) > 0
	f.Return.Enabled = !f.State.Busy()
	f.refreshServices()
}
func (f *Form) Update(in *seui.Input) {
	if f.ForgePending && !f.timeoutNotified && f.Now().Sub(f.ForgeAt) >= requestTimeout {
		f.timeoutNotified = true
		f.notice("ui.mall.timeout")
	}
	if f.State.Busy() && !f.timeoutNotified && f.Now().Sub(f.requestAt) >= requestTimeout {
		f.State.WaitingBalance = false
		// A sent purchase may already be committed. Keep it locked until its
		// receipts arrive or the connection resets; never enable blind retries.
		f.timeoutNotified = true
		f.notice("ui.mall.timeout")
		f.Refresh()
	}
	f.FixedForm.Update(in)
}
func (f *Form) text(x, y, width, lines int, value string) {
	data := clientassets.Big5Text(value)
	start, used, row := 0, 0, 0
	for i := 0; i < len(data); {
		n := 1
		if i+1 < len(data) && clientassets.Big5Index(data[i], data[i+1]) <= clientassets.MaxGlyph {
			n = 2
		}
		if used+n*8 > width || data[i] == '\n' {
			f.Env.Text.Draw(f.Left+x, f.Top+y+row*15, 0, false, true, f.Env.Screen, data[start:i], 15, width, 0, mallInk, 0)
			row++
			if row >= lines {
				return
			}
			start = i
			used = 0
			if data[i] == '\n' {
				i++
				start = i
				continue
			}
		}
		used += n * 8
		i += n
	}
	if start < len(data) && row < lines {
		f.Env.Text.Draw(f.Left+x, f.Top+y+row*15, 0, false, true, f.Env.Screen, data[start:], 15, width, 0, mallInk, 0)
	}
}
func (f *Form) pic(name string, x, y int) {
	f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(name), f.Left+x, f.Top+y, true)
}
func (f *Form) Hint() {
	if f.Blocked() {
		return
	}
	in := f.Env.UI.Input
	x, y := in.X-f.Left, in.Y-f.Top
	var entry *Entry
	if !f.InCart && !f.State.Bonus && f.Category == TabForging {
		gx, gy := x-forgeGridLeft, y-forgeGridTop
		if gx >= 0 && gx < forgeGridColumns*forgeCellStep && gy >= 0 && gy < game.BagRows*forgeCellStep {
			if slot := f.forgeOwner(byte(gy/forgeCellStep*forgeGridColumns + gx/forgeCellStep + 1)); slot != 0 {
				entry = &Entry{ID: f.forgeBag()[slot-1].ID}
			}
		}
	} else if f.InCart {
		row := (y - 37) / cartRowHeight
		if x >= 20 && x < 375 && y >= 37 && row < len(f.State.Cart) {
			entry = &f.State.Cart[row].Entry
		}
	} else {
		row := (y - 33) / catalogRowHeight
		rows := f.Entries()
		index := (f.Page-1)*CatalogRows + row
		if x >= 218 && x < 686 && y >= 33 && row < CatalogRows && index < len(rows) {
			entry = &rows[index]
		}
	}
	if entry != nil {
		n := f.Items[entry.ID]
		seui.WrappedTooltip(f.Env, in.X+16, in.Y+20, clientassets.Big5Text(n.Definition.Name+"\n"+n.Description), 0xffffff, 0x805020, 16, 36)
		return
	}
	f.FixedForm.Hint()
}
func (f *Form) item(e Entry, x, y int) {
	f.itemSized(e, x, y, nativeItemIconSize)
}
func (f *Form) itemSized(e Entry, x, y, size int) {
	id := f.Env.Pics.Find(strconv.Itoa(int(f.Items[e.ID].Icon)))
	w, h := f.Env.Pics.Size(id)
	if w <= 0 || h <= 0 {
		return
	}
	// Keep the larger catalog cell for centering without enlarging its
	// low-resolution artwork beyond the native inventory icon size.
	limit := min(size, nativeItemIconSize)
	if w > limit || h > limit {
		scale := float64(limit) / float64(max(w, h))
		w = max(1, int(float64(w)*scale))
		h = max(1, int(float64(h)*scale))
	}
	f.Env.Pics.DrawStretch(f.Env.Screen, id, image.Rect(f.Left+x+(size-w)/2, f.Top+y+(size-h)/2, f.Left+x+(size-w)/2+w, f.Top+y+(size-h)/2+h), true)
}
func (f *Form) Paint() {
	f.Env.Pics.Draw(f.Env.Screen, f.Image, f.Left, f.Top, true)
	if f.InCart {
		f.paintCart()
		return
	}
	f.pic("Form_Shopping_2", 218, 33)
	mark := "Icon_PointMark"
	if f.State.Bonus {
		mark = "Icon_BonusMark"
	}
	f.pic(mark, 71, 83)
	f.text(172, 126, 36, 1, strconv.FormatUint(uint64(f.State.Balances[0]), 10))
	f.text(172, 141, 36, 1, strconv.FormatUint(uint64(f.State.Balances[1]), 10))
	if f.Category == TabGames && !f.State.Bonus {
		f.paintCharacter()
		f.paintGames()
		return
	}
	if f.Category == TabForging && !f.State.Bonus {
		f.paintCharacter()
		f.paintForge()
		return
	}
	f.paintCharacter()
	rows := f.Entries()
	for i := 0; i < CatalogRows; i++ {
		index := (f.Page-1)*CatalogRows + i
		if index >= len(rows) {
			break
		}
		e := rows[index]
		y := catalogRowTop + i*catalogRowHeight
		n := f.Items[e.ID]
		f.itemSized(e, catalogItemX, y, catalogItemSize)
		name := n.Definition.Name
		if e.Count > 1 {
			name = fmt.Sprintf("%s ×%d", name, e.Count)
		}
		f.text(274, y+2, 98, 2, name)
		f.text(380, y+2, 225, 2, n.Description)
		f.pic("Icon_Price_1", 615, y+3)
		f.text(651, y+3, 35, 1, strconv.FormatUint(e.Price(), 10))
	}
	f.text(296, 523, 60, 1, fmt.Sprintf("%d / %d", f.Page, f.Pages()))
}
func (f *Form) paintCharacter() {
	s := f.Stats
	if s == nil {
		return
	}
	// Shared native status artwork, aligned to the mall's (-4,+6) offset.
	f.Env.Pics.DrawRect(f.Env.Screen, f.Env.Pics.Find("Form_RoleStatus_1"), f.Left+27, f.Top+334, image.Rect(15, 275, 188, 415), true)
	if f.PlayerName != nil {
		f.text(51, 170, 145, 1, string(f.PlayerName()))
	}
	f.pic(fmt.Sprintf("Icon_Element_%d_1", s.Element), 27, 165)
	f.status.Level(s.Level, image.Pt(f.Left+26, f.Top+198), true)
	if f.Preview != nil {
		f.Preview(f.Left+122, f.Top+previewFeetY, f.PreviewAction())
	}
	ratio := float64(0)
	if s.Formula != nil {
		ratio = s.Formula.Progress(s.Level, s.EXP, s.Rebirth != 0)
	}
	delta := f.FittingDelta()
	for i, v := range [][2]uint64{{uint64(s.HP), uint64(max(1, int64(s.MaxHP)+int64(delta.MaxHP)))}, {uint64(s.SP), uint64(max(1, int64(s.MaxSP)+int64(delta.MaxSP)))}, {uint64(ratio * 10000), 10000}} {
		y := 337 + i*14
		f.pic(fmt.Sprintf("Panel%d", 34+i), 54, 336+i*14)
		name := fmt.Sprintf("Panel%d", 31+i)
		id := f.Env.Pics.Find(name)
		w, h := f.Env.Pics.Size(id)
		if w > 0 && h > 0 {
			amount := int(min(v[0], max(1, v[1]))) * 56 / int(max(1, v[1]))
			f.Env.Pics.DrawRect(f.Env.Screen, id, f.Left+56, f.Top+y+1, image.Rect(0, 0, min(w, amount), min(h, 5)), true)
		}
	}
	f.fittingVital(111, 334, s.HP, s.MaxHP, delta.MaxHP)
	f.fittingVital(111, 348, uint32(s.SP), uint32(s.MaxSP), delta.MaxSP)
	f.text(111, 362, 94, 1, fmt.Sprintf("%.0f%%", ratio*100))
	f.text(51, 377, 70, 1, strconv.Itoa(int(s.Gold)))
	f.text(171, 377, 35, 1, strconv.Itoa(int(s.Points)))
	combat := s.CombatValues()
	bonuses := [5]int32{delta.ATK, delta.DEF, delta.MAT, delta.MDF, delta.SPD}
	for i, v := range combat {
		if bonuses[i] == 0 {
			f.text(56, 397+i*16, 57, 1, strconv.Itoa(int(v)))
		} else {
			f.fittingText(56, 397+i*16, fmt.Sprintf("%d%+d", v, bonuses[i]))
		}
	}
	for i, v := range []uint16{s.STR, s.CON, s.INT, s.WIS, s.AGI} {
		f.text(141, 397+i*16, 62, 1, strconv.Itoa(int(v)))
	}
}
func (f *Form) paintCart() {
	f.pic("Icon_ShopMark_2", 35, 463)
	for i, row := range f.State.Cart {
		y := cartRowTop + i*cartRowHeight
		n := f.Items[row.Entry.ID]
		f.item(row.Entry, 21, y+2)
		f.text(57, y-8, 72, 3, n.Definition.Name)
		f.text(133, y-8, 176, 3, n.Description)
		f.text(335, y+13, 60, 1, strconv.FormatUint(row.Entry.Price(), 10))
	}
	total := strconv.FormatUint(f.State.Total(), 10)
	f.text(505-len(total)*8, 463, 80, 1, total)
}
