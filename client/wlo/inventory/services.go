package inventory

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"fmt"
	"image"
	"strconv"
	"time"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// ServiceForm owns one session's native NPC service window. Inventory and
// currency remain server-confirmed; buttons only select or send requests.
type ServiceForm struct {
	LoadItem        func(uint16)
	PreviewPet      UsePet
	DrawPortrait    func(UsePet, image.Rectangle)
	DrawPet         func(UsePet, int, int)
	OnSubmit        func()
	SaleMode        byte
	PendingPetStats [][]byte
	Page            int
	CartSlots       [10]byte
	seui.FixedForm
	Inventory    *Form
	Mode         ServiceMode
	Storage      game.Inventory
	Hotel        [game.MaxHotelPets]UsePet
	Stock        []ShopEntry
	Selected     [2][game.BagSize]bool
	Cart         [10]byte
	Waiting      bool
	Send         func([]byte)
	Notice       func(string)
	Gold         func() uint32
	Owner        func() uint32
	OnClose      func()
	rows         []*serviceCell
	buttons      []*seui.FixedButton
	hotelSource  [game.MaxHotelPets]UsePet
	inputReadyAt time.Time
}
type ServiceMode byte

const (
	serviceOpeningInputDelay             = 250 * time.Millisecond
	ServiceBuy               ServiceMode = iota + 1
	ServiceSell
	ServiceStorage
	ServiceHotel
	serviceCellStep             = 34
	serviceGridTop              = 83 // FUN_001de750: storage grids.
	serviceGridLeft             = 20
	serviceGridRight            = 224
	serviceNoticeInk            = 0x0841
	serviceSalePriceOffset      = 118 // FUN_0036cd40: in-memory +0x7a, minus four-byte prefix.
	serviceSaleFlagsOffset      = 122
	serviceSaleUnavailable      = 16
	serviceHotelRowHeight       = 37 // FUN_00223208: native hotel row pitch.
	serviceHotelLeft            = 20
	serviceHotelRight           = 248
	serviceHotelTop             = 63
	serviceHotelWidth           = 203
	serviceHotelSelectionLeft   = 17 // Interior of the row borders in Form_Inn_1.
	serviceHotelSelectionRight  = 244
	serviceHotelSelectionWidth  = 201
	serviceHotelSelectionHeight = 33
	serviceHotelSelectedPaper   = 0xc6fe   // RGB565 (198,223,247), reference selected row.
	serviceHotelSelectedInk     = 0xfc00   // RGB565 (255,130,0), reference selected name.
	serviceTableHighlight       = 0xf5bf89 // Native TColor, not RGB565.
	serviceSelectionLight       = 0xff7f   // RGB565 (255,239,255), native selected grid scanline.
	serviceSelectionDark        = 0xf5df   // RGB565 (247,186,255).
)

const (
	serviceViewportWidth  = 800
	serviceViewportHeight = 600
)

type ShopEntry struct {
	Slot  byte
	ID    uint16
	Price uint32
}

func NewServiceForm(env *seui.Env, bag *Form) *ServiceForm {
	f := &ServiceForm{Inventory: bag}
	f.InitFixedForm(f, env)
	return f
}
func (f *ServiceForm) send(p []byte) {
	if f.Send != nil {
		f.Send(p)
	}
}
func (f *ServiceForm) notice(s string) {
	if f.Notice != nil {
		f.Notice(s)
	}
}
func (f *ServiceForm) Hide() {
	if d := f.Inventory.Dialog; d != nil && d.returnModal == f {
		d.Hide()
	}
	if f.Env != nil && f.Env.UI != nil && f.Env.UI.Modal == f {
		f.Env.UI.Modal = nil
	}
	f.FixedForm.Hide()
}
func (f *ServiceForm) Close() {
	f.Hide()
	f.Waiting = false
	if f.OnClose != nil {
		f.OnClose()
	}
}
func (f *ServiceForm) Reset() {
	f.Hide()
	f.Storage = game.Inventory{}
	f.Hotel = [game.MaxHotelPets]UsePet{}
	f.hotelSource = [game.MaxHotelPets]UsePet{}
	f.CartSlots = [10]byte{}
	f.Stock = nil
	f.Selected = [2][game.BagSize]bool{}
	f.Cart = [10]byte{}
	f.Waiting = false
	f.PendingPetStats = nil
	f.PreviewPet = UsePet{}
}
func (f *ServiceForm) button(pic string, x, y int, click func()) {
	b := seui.NewFixedButton(f.Env, f)
	w, h := 56, 20
	switch pic {
	case "Btn_ArrowL_8", "Btn_ArrowR_8":
		w, h = 21, 21
	case "Btn_ArrowL_5", "Btn_ArrowR_5":
		w, h = 17, 17
	case "Btn_Close_s_1":
		w, h = 18, 18
	}
	b.Init(pic, x, h, w, 0, 0, true, h, w, y)
	b.OnClick = func() {
		if f.inputReady() {
			click()
		}
	}
	f.buttons = append(f.buttons, b)
}
func (f *ServiceForm) Open(mode ServiceMode) {
	f.inputReadyAt = f.now().Add(serviceOpeningInputDelay)
	for _, r := range f.rows {
		r.Visible = false
	}
	for _, b := range f.buttons {
		b.Visible = false
	}
	if f.Env != nil && f.Env.UI != nil {
		in := f.Env.UI.Input
		for _, p := range []*seui.Control{&in.Pressed, &in.Captured, &in.Focused, &in.Hovered} {
			if *p != nil && (*p).Base().Root() == f {
				*p = nil
			}
		}
	}
	f.Children = nil
	f.rows = nil
	f.buttons = nil
	f.Selected = [2][game.BagSize]bool{}
	f.Cart = [10]byte{}
	f.Waiting = false
	f.Mode = mode
	f.PreviewPet = UsePet{}
	f.Page = 0
	f.CartSlots = [10]byte{}
	art, w, h := "Form_itemBank_1", 421, 481
	switch mode {
	case ServiceSell:
		art, w, h = "Form_SaleThing_1", 241, 505
		if f.SaleMode == 0 {
			art = "Form_SaleEquip_1"
			h = 505
		}
	case ServiceBuy:
		art, w, h = "Form_ThingStore_1", 409, 525
	case ServiceHotel:
		art, w, h = "Form_Inn_1", 472, 476
	}
	f.Init(art, (serviceViewportWidth-w)/2, 0, 0, 0, 0, false, h, w, (serviceViewportHeight-h)/2)
	// FUN_00368f10 / FUN_0036c60c / FUN_001de750 / FUN_00223208.
	// The native backgrounds are fixed bitmaps, not nine-slice panels.
	switch mode {
	case ServiceBuy:
		f.button("Btn_Close_1", 214, 489, f.Close)
		f.button("Btn_Close_s_1", 358, 25, f.Close)
		f.button("Btn_Buy_1", 136, 489, f.Confirm)
	case ServiceSell:
		y := 461
		if f.SaleMode == 0 {
			y = 471
		}
		f.button("Btn_Close_1", 129, y, f.Close)
		f.button("Btn_Close_s_1", 191, 26, f.Close)
		f.button("Btn_confirm_1", 54, y, f.Confirm)
	case ServiceStorage:
		f.button("Btn_Close_1", 179, 437, f.Close)
		f.button("Btn_Close_s_1", 365, 25, f.Close)
		f.button("Btn_ArrowR_5", 200, 200, func() { f.Transfer(false) })
		f.button("Btn_ArrowL_5", 199, 247, func() { f.Transfer(true) })
	case ServiceHotel:
		f.button("Btn_Close_1", 208, 440, f.Close)
		f.button("Btn_Close_s_1", 422, 24, f.Close)
		f.button("Btn_ArrowR_8", 221, 108, func() { f.Transfer(false) })
		f.button("Btn_ArrowL_8", 221, 144, func() { f.Transfer(true) })
	}
	switch mode {
	case ServiceBuy:
		for i := 0; i < 5; i++ {
			f.cell(0, i, 26, 82+i*59, 353, 56)
		}
		for i := range f.Cart {
			f.cell(1, i, 34+i*34, 402, 32, 32)
		}
	case ServiceHotel:
		for i := 0; i < game.MaxHotelPets; i++ {
			f.cell(0, i, serviceHotelLeft, serviceHotelTop+i*serviceHotelRowHeight, serviceHotelWidth, 34)
		}
		for i := 0; i < game.MaxPets; i++ {
			f.cell(1, i, serviceHotelRight, serviceHotelTop+i*serviceHotelRowHeight, serviceHotelWidth, 34)
		}
	default:
		sides := 2
		if mode == ServiceSell {
			sides = 1
		}
		for side := 0; side < sides; side++ {
			for i := 0; i < game.BagSize; i++ {
				x := serviceGridLeft
				if side == 1 {
					x = serviceGridRight
				}
				if mode == ServiceSell {
					x = 35
				}
				top := serviceGridTop
				if mode == ServiceSell {
					top = 69
				} // FUN_003638dc(35, 69).
				f.cell(side, i, x+i%BagColumns*34, top+i/BagColumns*34, 32, 32)
			}
		}
	}
	if f.LoadItem != nil {
		for _, e := range f.Stock {
			f.LoadItem(e.ID)
		}
		for _, bag := range []game.Inventory{f.Storage, f.Inventory.State.Bag} {
			for _, it := range bag {
				if !it.Empty() {
					f.LoadItem(it.ID)
				}
			}
		}
	}
	f.Show()
	f.Env.UI.Modal = f
}
func (f *ServiceForm) cell(side, index, x, y, w, h int) {
	r := &serviceCell{form: f, side: side, index: index}
	r.InitComponent(r, f.Env, f)
	r.Init("", x, 0, 0, 0, 0, false, h, w, y)
	f.rows = append(f.rows, r)
}
func (f *ServiceForm) text(x, y, w int, s string) {
	f.Env.Text.Draw(f.Left+x, f.Top+y, 0, false, true, f.Env.Screen, []byte(s), 13, w, 0, serviceNoticeInk, 0)
}
func (f *ServiceForm) Paint() {
	f.FixedForm.Paint()
	if f.Mode == ServiceHotel {
		f.PreviewPet = UsePet{}
		if f.Env.UI != nil {
			for _, row := range f.rows {
				if row.HitTest(f.Env.UI.Input.X, f.Env.UI.Input.Y) && !row.Blocked() {
					f.PreviewPet, _ = row.hotelPet()
					break
				}
			}
		}
		title := f.Env.Pics.Find("Icon_Inn_1")
		tw, _ := f.Env.Pics.Size(title)
		f.Env.Pics.Draw(f.Env.Screen, title, f.Left+(f.Width-tw)/2, f.Top+11, true)
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Bmp_Friend_1"), f.Left+255, f.Top+408, true)
		p := f.PreviewPet
		if p.ID != 0 {
			if f.DrawPet != nil {
				f.DrawPet(p, f.Left+347, f.Top+320)
			}
			f.text(253, 355, 160, string(p.Name))
			f.text(273, 373, 100, fmt.Sprintf("%d", p.Stats.Level))
			f.text(273, 390, 130, fmt.Sprintf("%d/%d", p.Stats.HP, p.Stats.MaxHP))
			f.text(298, 407, 80, fmt.Sprintf("%d", p.Amity))
		}
	}
	for _, b := range f.buttons {
		b.Enabled = !f.Waiting
	}
	if f.Mode == ServiceBuy || f.Mode == ServiceSell {
		x, y := 189, f.Height-85
		if f.Mode == ServiceBuy {
			x, y = 268, 449
		}
		f.text(x, y, 80, fmt.Sprintf("%d", f.Total()))
		if f.Gold != nil {
			f.text(x, y+22, 80, fmt.Sprintf("%d", f.Gold()))
		}
	}
}
func (f *ServiceForm) Total() uint64 {
	var n uint64
	if f.Mode == ServiceBuy {
		for i, c := range f.Cart {
			n += uint64(c) * uint64(f.cartEntry(i).Price)
		}
	} else {
		for i, s := range f.Selected[0] {
			if s {
				it := f.Inventory.State.Bag[i]
				n += uint64(f.SalePrice(it.ID)) * uint64(it.Count)
			}
		}
	}
	return n
}
func (f *ServiceForm) SalePrice(id uint16) uint32 {
	it := f.Inventory.State.Items[id]
	return binary.LittleEndian.Uint32(it.Record[serviceSalePriceOffset:])
}
func (f *ServiceForm) Confirm() {
	if f.Waiting {
		return
	}
	var p []byte
	if f.Mode == ServiceBuy {
		if f.Gold != nil && f.Total() > uint64(f.Gold()) {
			f.notice(localization.UI("ui.inventory.not_enough_gold_c1d5d70f"))
			return
		}
		p = []byte{protocol.CommandShop, protocol.ShopBuy, 0}
		for i, c := range f.Cart {
			if c > 0 && f.cartEntry(i).ID != 0 {
				p = append(p, f.CartSlots[i], c)
				p[2]++
			}
		}
		if p[2] == 0 {
			return
		}
	} else if f.Mode == ServiceSell {
		p = []byte{protocol.CommandShop, protocol.ShopSell}
		for i, s := range f.Selected[0] {
			if s {
				p = append(p, byte(i+1))
			}
		}
		if len(p) == 2 {
			return
		}
		p = append(p, f.SaleMode)
	} else {
		return
	}
	f.Waiting = true
	f.send(p)
	if f.Mode == ServiceBuy && f.OnSubmit != nil {
		f.OnSubmit()
	}
}
func (f *ServiceForm) Transfer(deposit bool) {
	if f.Waiting {
		return
	}
	side := 0
	if deposit {
		side = 1
	}
	command, sub := byte(protocol.CommandStorage), byte(protocol.StorageWithdraw)
	if deposit {
		sub = protocol.StorageDeposit
	}
	if f.Mode == ServiceHotel {
		command = protocol.CommandNPCService
		sub = protocol.PetHotelWithdraw
		if deposit {
			sub = protocol.PetHotelDeposit
		}
	}
	p := []byte{command, sub}
	for i, s := range f.Selected[side] {
		if s {
			p = append(p, byte(i+1))
		}
	}
	if len(p) == 2 {
		return
	}
	f.Waiting = true
	f.send(p)
}
func (f *ServiceForm) Select(side, index int) {
	if f.Waiting {
		return
	}
	if f.Mode == ServiceBuy {
		if side == 0 {
			index += f.Page
			if index < 0 || index >= len(f.Stock) {
				return
			}
			slot := f.Stock[index].Slot
			for i := range f.Cart {
				if f.CartSlots[i] == slot && f.Cart[i] < game.MaxItemStack {
					f.Cart[i]++
					return
				}
			}
			for i := range f.Cart {
				if f.Cart[i] == 0 {
					f.CartSlots[i] = slot
					f.Cart[i] = 1
					return
				}
			}
		} else if index >= 0 && index < len(f.Cart) {
			f.Cart[index] = 0
			f.CartSlots[index] = 0
		}
		return
	}
	if side < 0 || side > 1 || index < 0 || index >= game.BagSize {
		return
	}
	if f.Mode == ServiceHotel {
		limit := game.MaxHotelPets
		if side == 1 {
			limit = game.MaxPets
		}
		if index >= limit {
			return
		}
		pet := f.Hotel[index]
		if side == 1 {
			pet = f.Inventory.State.Pets[index]
		}
		if pet.ID == 0 {
			return
		}
	} else {
		bag := &f.Storage
		if side == 1 || f.Mode == ServiceSell {
			bag = &f.Inventory.State.Bag
		}
		state := *f.Inventory.State
		state.Bag = *bag
		anchor := state.BagAnchor(index + 1)
		if anchor == 0 {
			return
		}
		index = anchor - 1
		it := bag[index]
		if it.Empty() || it.Locked {
			return
		}
		if f.Mode == ServiceSell {
			n := f.Inventory.State.Items[it.ID]
			if n.Record[serviceSaleFlagsOffset]&serviceSaleUnavailable != 0 || (f.SaleMode == 0) != (n.Definition.EquipSlot >= 1 && n.Definition.EquipSlot <= EquipmentSlots) {
				return
			}
		}
	}
	f.Selected[side][index] = !f.Selected[side][index]
}

type serviceCell struct {
	seui.Component
	form        *ServiceForm
	side, index int
}

func (r *serviceCell) LeftUp(shift byte, x, y int) {
	if r.form.inputReady() && r.HitTest(x, y) && !r.Blocked() {
		if shift&controlSplit != 0 && r.side == 0 && r.form.Mode == ServiceBuy && r.form.buyQuantity(r.index+r.form.Page) {
			return
		}
		index := r.index
		if r.form.Mode == ServiceHotel {
			_, slot := r.hotelPet()
			if slot == 0 {
				return
			}
			index = slot - 1
		}
		r.form.Select(r.side, index)
	}
}

func (f *ServiceForm) now() time.Time {
	if f.Inventory.Now != nil {
		return f.Inventory.Now()
	}
	return time.Now()
}

// Ignore clicks carried across from the dialogue that opened this service.
// Hovering and rendering remain responsive during the short opening delay.
func (f *ServiceForm) inputReady() bool {
	return !f.now().Before(f.inputReadyAt)
}

// Ctrl-click adds a chosen quantity to the cart, without spending or granting
// anything until the normal Buy request succeeds.
func (f *ServiceForm) buyQuantity(index int) bool {
	if f.Waiting || index < 0 || index >= len(f.Stock) {
		return true
	}
	entry := f.Stock[index]
	limit := f.Inventory.State.Items[entry.ID].Definition.StackLimit()
	if limit <= 1 {
		return false
	}
	row := -1
	for i, slot := range f.CartSlots {
		if slot == entry.Slot && f.Cart[i] > 0 && f.Cart[i] < limit {
			row = i
			break
		}
	}
	if row < 0 {
		for i, count := range f.Cart {
			if count == 0 {
				row = i
				break
			}
		}
	}
	if row < 0 || f.Cart[row] >= limit {
		return true
	}
	previous := f.Cart[row]
	f.Inventory.quantity(localization.UI("ui.inventory.purchase_quantity_b9604aa0"), limit-previous, func(count byte) {
		if !f.Visible || f.Mode != ServiceBuy || f.Waiting || index >= len(f.Stock) || f.Stock[index] != entry || f.Cart[row] != previous {
			return
		}
		f.CartSlots[row] = entry.Slot
		f.Cart[row] += count
	})
	return true
}
func (r *serviceCell) item() game.Item {
	f := r.form
	if f.Mode == ServiceBuy {
		if r.side == 1 {
			e := f.cartEntry(r.index)
			return game.Item{ID: e.ID, Count: f.Cart[r.index]}
		}
		i := r.index + f.Page
		if i < len(f.Stock) {
			return game.Item{ID: f.Stock[i].ID, Count: 1}
		}
		return game.Item{}
	}
	bag := f.Storage
	if r.side == 1 || f.Mode == ServiceSell {
		bag = f.Inventory.State.Bag
	}
	return bag[r.index]
}

// FUN_002242d0 packs occupied pets into display rows. Requests still use
// their original storage/team slots, never their displayed row numbers.
func (r *serviceCell) hotelPet() (UsePet, int) {
	row := 0
	visit := func(p UsePet) bool {
		if p.ID == 0 {
			return false
		}
		if row == r.index {
			return true
		}
		row++
		return false
	}
	if r.side == 0 {
		for i, p := range r.form.Hotel {
			if visit(p) {
				return p, i + 1
			}
		}
	} else {
		for i, p := range r.form.Inventory.State.Pets {
			if visit(p) {
				return p, i + 1
			}
		}
	}
	return UsePet{}, 0
}

func (r *serviceCell) Paint() {
	f := r.form
	a := r.Abs()
	if f.Mode == ServiceHotel {
		pet, slot := r.hotelPet()
		if slot == 0 {
			return
		}
		selected := f.Selected[r.side][slot-1]
		if selected {
			x := f.Left + serviceHotelSelectionLeft
			if r.side == 1 {
				x = f.Left + serviceHotelSelectionRight
			}
			f.Env.Screen.Fill(image.Rect(x, a.Y, x+serviceHotelSelectionWidth, a.Y+serviceHotelSelectionHeight), serviceHotelSelectedPaper)
		}
		if f.DrawPortrait != nil {
			f.DrawPortrait(pet, image.Rect(a.X+2, a.Y+2, a.X+26, a.Y+26))
		}
		ink := uint16(0x001f)
		if selected {
			ink = serviceHotelSelectedInk
		} // Native selected row uses orange name ink.
		f.Env.Text.Draw(a.X+34, a.Y+7, 0, false, true, f.Env.Screen, pet.Name, 15, 80, 0, ink, 0)
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(fmt.Sprintf("Icon_Element_%d_1", pet.Stats.Element)), a.X+115, a.Y+3, true)
		r.petVitals(pet)
		return
	}
	it := r.item()
	if it.Empty() {
		return
	}
	if f.Selected[r.side][r.index] {
		w, h := f.Inventory.State.Items[it.ID].Definition.Footprint()
		bounds := image.Rect(a.X, a.Y, a.X+w*serviceCellStep-2, a.Y+h*serviceCellStep-2)
		// Native selected.png: alternating pale pink scanlines; keep the
		// two-pixel grid gutters clear, including multi-slot footprints.
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			ink := uint16(serviceSelectionLight)
			if (y-bounds.Min.Y)%2 != 0 {
				ink = serviceSelectionDark
			}
			f.Env.Screen.Fill(image.Rect(bounds.Min.X, y, bounds.Max.X, y+1), ink)
		}
	}
	if f.Mode == ServiceBuy && r.side == 0 {
		if f.Env.UI != nil && image.Pt(f.Env.UI.Input.X, f.Env.UI.Input.Y).In(r.Rect()) && !r.Blocked() {
			f.Env.Screen.FillAlpha(r.Rect(), serviceTableHighlight, 64)
		}
		n := f.Inventory.State.Items[it.ID]
		// Native table cells: icon 32x56, name 72x56, price 61x56.
		f.Inventory.drawItem(it, a.X, a.Y+12, false)
		r.tableText(35, 72, n.Definition.Name, true)
		r.tableText(292, 61, strconv.FormatUint(uint64(f.Stock[r.index+f.Page].Price), 10), true)
		effects := itemEffectLines(n.Definition)
		if len(effects) == 0 {
			effects = []string{n.Description}
		}
		row := 0
		for _, effect := range effects {
			for _, line := range serviceTextLines(effect, 176) {
				if row == 3 {
					break
				}
				f.Env.Text.Draw(a.X+111, a.Y+3+row*15, 0, false, true, f.Env.Screen, line, 15, 176, 0, serviceNoticeInk, 0)
				row++
			}
		}
	} else {
		f.Inventory.drawItem(it, a.X, a.Y, true)
	}
}

func (r *serviceCell) tableText(x, width int, value string, center bool) {
	f := r.form
	lines := serviceTextLines(value, width)
	lines = lines[:min(3, len(lines))]
	y := 3
	if len(clientassets.Big5Text(value))*8 < width {
		y = (r.Height - 15) / 2
	}
	for i, line := range lines {
		left := x
		if len(lines) == 1 && center {
			left += (width - len(line)*8) / 2
		}
		f.Env.Text.Draw(f.Left+r.Left+left, f.Top+r.Top+y+i*15, 0, false, true, f.Env.Screen, line, 15, width, 0, serviceNoticeInk, 0)
	}
}

// Preserve Big5 character boundaries and keep table text inside its cell.
func serviceTextLines(value string, width int) [][]byte {
	data := clientassets.Big5Text(value)
	var lines [][]byte
	start, used := 0, 0
	for i := 0; i < len(data); {
		count := 1
		if i+1 < len(data) && clientassets.Big5Index(data[i], data[i+1]) <= clientassets.MaxGlyph {
			count = 2
		}
		if used+count*8 > width {
			lines = append(lines, data[start:i])
			start, used = i, 0
		}
		used += count * 8
		i += count
	}
	return append(lines, data[start:])
}

// FUN_0029d3a4, with the hotel offsets from FUN_00223208. Reuse the
// native tiny labels, digit strip and framed gauges instead of full-size text.
func (r *serviceCell) petVitals(p UsePet) {
	f := r.form
	a := r.Abs()
	x, y := a.X+148, a.Y+2
	f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Word_Lv_1"), x, y+2, true)
	digits := f.Env.Pics.Find("Pet_Num_Lv_Black_1")
	w, h := f.Env.Pics.Size(digits)
	h /= 10
	for i, digit := range strconv.Itoa(int(p.Stats.Level)) {
		top := int(digit-'0') * h
		f.Env.Pics.DrawRect(f.Env.Screen, digits, x+17+i*w, y+2, image.Rect(0, top, w, top+h), true)
	}
	for i, kind := range []string{"Hp", "Sp"} {
		top := y + 10 + i*8
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Word_"+kind+"_1"), x, top, true)
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Pet_Frame_"+kind), x+13, top+1, true)
		value, maximum := p.Stats.HP, p.Stats.MaxHP
		if i == 1 {
			value, maximum = uint32(p.Stats.SP), uint32(p.Stats.MaxSP)
		}
		if maximum > 0 {
			bar := f.Env.Pics.Find("Pet_Bmp_" + kind)
			w, h := f.Env.Pics.Size(bar)
			w = int(uint64(w) * uint64(min(value, maximum)) / uint64(maximum))
			f.Env.Pics.DrawRect(f.Env.Screen, bar, x+14, top+2, image.Rect(0, 0, w, h), true)
		}
	}

}
func (r *serviceCell) Hint() {
	if r.form.Mode == ServiceHotel {
		return
	}
	it := r.item()
	if it.Empty() && r.form.Mode != ServiceBuy {
		state := *r.form.Inventory.State
		if r.side == 0 && r.form.Mode == ServiceStorage {
			state.Bag = r.form.Storage
		}
		if anchor := state.BagAnchor(r.index + 1); anchor != 0 {
			it = state.Bag[anchor-1]
		}
	}
	if it := it; !it.Empty() && r.form.Mode != ServiceHotel {
		r.form.Inventory.drawItemHint(&r.Component, it)
	}
}

// Storage packets share item record layouts with the bag. Decode on a copy.
func (f *ServiceForm) StoragePacket(p []byte) bool {
	if len(p) < 2 {
		return false
	}
	switch p[1] {
	case protocol.StorageOpen:
		f.Storage = game.Inventory{}
		f.Open(ServiceStorage)
	case protocol.StorageItems:
		copy := *f.Inventory.State
		copy.Bag = f.Storage
		q := append([]byte{protocol.CommandInventory, protocol.InventoryItems}, p[2:]...)
		if _, ok := copy.Apply(q); !ok {
			return false
		}
		f.Storage = copy.Bag
		if f.LoadItem != nil {
			for _, it := range f.Storage {
				if !it.Empty() {
					f.LoadItem(it.ID)
				}
			}
		}
	case protocol.StorageAdd:
		copy := *f.Inventory.State
		copy.Bag = f.Storage
		q := append([]byte{protocol.CommandInventory, protocol.InventoryAcquire}, p[2:]...)
		if _, ok := copy.Apply(q); !ok {
			return false
		}
		f.Storage = copy.Bag
		if f.LoadItem != nil {
			for _, it := range f.Storage {
				if !it.Empty() {
					f.LoadItem(it.ID)
				}
			}
		}
	case protocol.StorageRemove:
		if len(p) != 4 {
			return false
		}
		copy := f.Storage
		if copy.Remove(p[2], p[3]) != nil {
			return false
		}
		f.Storage = copy
	case protocol.StorageDepositUnlock, protocol.StorageWithdrawUnlock:
		f.Waiting = false
		f.Selected = [2][game.BagSize]bool{}
		f.PreviewPet = UsePet{}
	default:
		return false
	}
	return true
}

// Hotel copy messages move an existing record; subsequent roster/stat messages
// can enrich it without a speculative local transfer when the button is clicked.
func (f *ServiceForm) HotelPacket(p []byte) bool {
	if len(p) < 2 {
		return false
	}
	switch p[1] {
	case protocol.PetHotelOpen:
		f.Open(ServiceHotel)
	case protocol.NPCServiceWireCode3:
		if len(p) != 4 || p[2] < 1 || p[2] > game.MaxHotelPets || p[3] < 1 || p[3] > game.MaxPets {
			return false
		}
		f.Hotel[p[2]-1] = f.Inventory.State.Pets[p[3]-1]
		f.hotelSource[p[2]-1] = f.Hotel[p[2]-1]
		f.notice(string(f.Hotel[p[2]-1].Name) + localization.UI("ui.inventory.left_party_502d6b59"))
	case protocol.PetHotelCopyToTeam:
		if len(p) != 4 || p[2] < 1 || p[2] > game.MaxHotelPets || p[3] < 1 || p[3] > game.MaxPets {
			return false
		}
		pet := f.Hotel[p[2]-1]
		if pet.ID == 0 {
			pet = f.hotelSource[p[2]-1]
		}
		if pet.ID != 0 {
			current := f.Inventory.State.Pets[p[3]-1]
			if current.ID == pet.ID {
				if len(current.Name) == 0 {
					current.Name = pet.Name
				}
				pet = current
			}
			f.Inventory.State.Pets[p[3]-1] = pet
			for _, q := range f.PendingPetStats {
				f.Inventory.State.ApplyPetStat(q)
			}
			f.PendingPetStats = nil
			f.notice(string(pet.Name) + localization.UI("ui.inventory.joined_party_3c80fab6"))
		}
	case protocol.NPCServiceWireCode4:
		if len(p) != 3 || p[2] < 1 || p[2] > game.MaxHotelPets {
			return false
		}
		f.Hotel[p[2]-1] = UsePet{}
	case protocol.NPCServiceWireCode6:
		return f.hotelList(p)
	case protocol.PetHotelStorageUnlock, protocol.PetHotelDepositUnlock, protocol.PetHotelModalUnlock:
		f.Waiting = false
		f.Selected = [2][game.BagSize]bool{}
		f.PreviewPet = UsePet{}
	default:
		return false
	}
	return true
}
func (f *ServiceForm) hotelList(p []byte) bool {
	const prefix = 30
	const tail = 34
	next := f.Hotel
	for at := 2; at < len(p); {
		if len(p)-at < prefix {
			return false
		}
		r := p[at:]
		slot := r[0]
		n := int(r[29])
		size := prefix + n + tail
		if slot < 1 || slot > game.MaxHotelPets || n > 10 || len(r) < size {
			return false
		}
		pet := UsePet{ID: binary.LittleEndian.Uint16(r[1:]), Name: append([]byte(nil), r[prefix:prefix+n]...)}
		pet.Stats.EXP = binary.LittleEndian.Uint32(r[3:])
		pet.Stats.Level = r[7]
		pet.Stats.HP = binary.LittleEndian.Uint32(r[8:])
		pet.Stats.SP = binary.LittleEndian.Uint16(r[12:])
		pet.Stats.INT = binary.LittleEndian.Uint16(r[14:])
		pet.Stats.STR = binary.LittleEndian.Uint16(r[16:])
		pet.Stats.CON = binary.LittleEndian.Uint16(r[18:])
		pet.Stats.AGI = binary.LittleEndian.Uint16(r[20:])
		pet.Stats.WIS = binary.LittleEndian.Uint16(r[22:])
		pet.Stats.Rebirth = r[prefix+n+27]
		pet.Stats.Job = r[prefix+n+29]
		pet.Potential = r[prefix+n+28]
		pet.Stats.Potential = pet.Potential
		pet.Amity = r[25]
		pet.Stats.Points = binary.LittleEndian.Uint16(r[27:])
		if pet.Potential > game.PotentialMaximum {
			return false
		}
		for i := range pet.Skills {
			at := prefix + n + i*5
			if r[at] > f.Inventory.State.petSkillGradeLimit(pet.ID, i) {
				return false
			}
			pet.Skills[i] = game.PetSkill{Grade: r[at], Exp: binary.LittleEndian.Uint32(r[at+1:])}
		}
		for i := range pet.Equipment {
			at := prefix + n + 15 + i*2
			if id := binary.LittleEndian.Uint16(r[at:]); id != 0 {
				pet.Equipment[i] = game.Item{ID: id, Count: 1}
			}
		}
		if pet.ID == 0 {
			return false
		}
		f.Inventory.State.recomputePet(&pet)
		next[slot-1] = pet
		at += size
	}
	f.Hotel = next
	return true
}

// Named constructor kept here to make catalog ownership explicit.
func ShopStock(items map[uint16]assets.NativeItem, entries []ShopEntry) []ShopEntry {
	out := append([]ShopEntry(nil), entries...)
	for i := range out {
		n := items[out[i].ID]
		out[i].Price = n.BuyPrice()
	}
	return out
}

func (f *ServiceForm) cartEntry(index int) ShopEntry {
	for _, e := range f.Stock {
		if e.Slot == f.CartSlots[index] {
			return e
		}
	}
	return ShopEntry{}
}
func (f *ServiceForm) WheelUp() {
	if f.Mode == ServiceBuy {
		f.Page = max(0, f.Page-1)
	}
}
func (f *ServiceForm) WheelDown() {
	if f.Mode == ServiceBuy {
		f.Page = min(max(0, len(f.Stock)-5), f.Page+1)
	}
}
