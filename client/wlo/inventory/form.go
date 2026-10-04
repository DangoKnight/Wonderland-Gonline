package inventory

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/world"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// Coordinates and controls from FUN_0034fad4, FUN_003638dc, FUN_00356214
// and FUN_0035172c, WLRI ca19ee087b60. The capture is a visual check only.
const (
	formWidth          = 386
	formHeight         = 461
	gridLeft           = 193
	gridTop            = 74
	cellStep           = 34
	cellSize           = 32
	previewX           = 100
	previewY           = 211
	previewBattleReady = 17 // FUN_00353384 sets the copied player to action 0x11
	controlSplit       = 1 << 2
	inventoryTextInk   = uint16(0x0841)
	inventoryTextPaper = uint16(0)
	dragThreshold      = 4
)

var equipmentPoints = [EquipmentSlots]image.Point{{84, 84}, {147, 186}, {25, 125}, {147, 126}, {84, 220}, {24, 185}}

// Form is TSe_EquipForm2: server-confirmed bag, worn slots and a separate
// character preview. Mode 0 is combined, 1 status only, 2 inventory only.
type Form struct {
	seui.Form
	State                                                           *State
	Stats                                                           *world.Stats
	PlayerName                                                      []byte
	Preview                                                         login.RoleView
	Status                                                          *login.Status
	Send                                                            func([]byte)
	CanAct                                                          func() bool
	Notice                                                          func(string)
	Close, CloseTitle, RotateLeft, RotateRight, StatusOnly, BagOnly *seui.FixedButton
	Slots                                                           [game.BagSize]*slotControl
	Worn                                                            [EquipmentSlots]*slotControl
	Mode                                                            byte
	drag                                                            *dragItem
	itemInfo                                                        *seui.Panel
	Dialog                                                          *actionDialog
}
type dragItem struct {
	slot  int
	worn  bool
	item  game.Item
	start image.Point
	moved bool
	split bool
}
type slotControl struct {
	seui.Component
	form *Form
	slot int
	worn bool
}

func NewForm(env *seui.Env, state *State, stats *world.Stats) *Form {
	f := &Form{State: state, Stats: stats}
	f.InitForm(f, env)
	f.Name = "TSe_EquipForm2"
	f.Dockable = false
	f.Init("Form_RoleStatus_1", (800-formWidth)/2, 0, 0, 0, 0, false, formHeight, formWidth, (600-formHeight)/2)
	f.Status = login.NewStatus(env, stats.Formula)
	f.CloseTitle = f.button("Btn_Close_s_1", 336, 26, 18, 18, func() { f.Hide() })
	f.Close = f.button("Btn_Close_1", (formWidth-56)/2, 423, 56, 20, func() { f.Hide() })
	f.Close.SetHint([]byte("[ESC]"))
	f.RotateLeft = f.button("Btn_ArrowL_9", 40, 245, 20, 20, nil)
	f.RotateRight = f.button("Btn_ArrowR_9", 139, 245, 20, 20, nil)
	// Native callbacks select the previous/next party pet, keeping action 17;
	// they do not rotate the player. Pet inventory views remain unported.
	f.RotateLeft.Enabled, f.RotateRight.Enabled = false, false
	f.StatusOnly = f.button("Btn_ArrowL_5", 149, 30, 17, 17, func() {
		if f.Mode == 0 {
			f.SetMode(1)
		} else {
			f.SetMode(0)
		}
	})
	f.BagOnly = f.button("Btn_ArrowR_5", 222, 30, 17, 17, func() {
		if f.Mode == 0 {
			f.SetMode(2)
		} else {
			f.SetMode(0)
		}
	})
	for i := range f.Slots {
		f.Slots[i] = f.slot(i+1, false)
	}
	for i := range f.Worn {
		f.Worn[i] = f.slot(i+1, true)
	}
	f.SetMode(0)
	return f
}
func (f *Form) button(name string, x, y, w, h int, click func()) *seui.FixedButton {
	b := seui.NewFixedButton(f.Env, f)
	b.Init(name, x, h, w, 0, 0, true, h, w, y)
	b.OnClick = click
	return b
}
func (f *Form) slot(n int, worn bool) *slotControl {
	c := &slotControl{form: f, slot: n, worn: worn}
	c.InitComponent(c, f.Env, f)
	c.Init("", 0, 0, 0, 0, 0, false, cellSize, cellSize, 0)
	c.DownSound = 0xff
	return c
}
func (f *Form) SetMode(mode byte) {
	if mode > 2 {
		return
	}
	f.drag = nil
	oldRight := f.Right()
	oldMode := f.Mode
	f.Mode = mode
	names := [3]string{"Form_RoleStatus_1", "Form_RoleStatus_2", "Form_RoleStatus_3"}
	widths := [3]int{386, 207, 209}
	if mode == 2 || mode == 0 && oldMode == 2 {
		f.Left = oldRight - widths[mode]
	}
	f.Width = widths[mode]
	f.Image = f.Env.Pics.Find(names[mode])
	f.Left = max(0, min(800-f.Width, f.Left))
	f.Close.Left = (f.Width - 56) / 2
	f.CloseTitle.Left = f.Width - 50
	f.StatusOnly.Left = 149
	f.BagOnly.Left = 222
	f.StatusOnly.SetVisible(mode != 1)
	f.BagOnly.SetVisible(mode != 2)
	if mode == 1 {
		f.BagOnly.Left = 125
	}
	if mode == 2 {
		f.StatusOnly.Left = 60
	}
	f.StatusOnly.SetHint([]byte("Status only / combined"))
	f.BagOnly.SetHint([]byte("Inventory only / combined"))
	f.RotateLeft.SetVisible(mode != 2)
	f.RotateRight.SetVisible(mode != 2)
	for i, c := range f.Slots {
		c.SetVisible(mode != 1)
		left := gridLeft
		if mode == 2 {
			left = 19
		}
		c.SetPos(left+(i%BagColumns)*cellStep, gridTop+(i/BagColumns)*cellStep)
	}
	for i, c := range f.Worn {
		c.SetVisible(mode != 2)
		c.SetPos(equipmentPoints[i].X, equipmentPoints[i].Y)
	}
}
func (f *Form) Hide() {
	f.drag = nil
	if f.Dialog != nil {
		f.Dialog.Hide()
	}
	f.Form.Hide()
}
func (f *Form) KeyDown(key uint16, shift byte) {
	if key == 0x1b {
		f.Hide()
	}
}
func (f *Form) Paint() {
	f.Env.Pics.Draw(f.Env.Screen, f.Image, f.Left, f.Top, true)
	if f.Mode == 2 {
		return
	}
	at := func(x, y int) image.Point { return image.Pt(f.Left+x, f.Top+y) }
	f.label(49, 61, string(f.PlayerName))
	f.Status.Level(f.Stats.Level, at(20, 88), true)
	element := f.Env.Pics.Find(fmt.Sprintf("Icon_Element_%d_1", f.Stats.Element))
	f.Env.Pics.Draw(f.Env.Screen, element, f.Left+19, f.Top+56, true)
	f.Env.Pics.DrawRect(f.Env.Screen, f.Env.Pics.Find("Btn_Potential_1"), f.Left+126, f.Top+88, image.Rect(0, 0, 43, 20), true)
	f.label(172, 90, strconv.Itoa(int(f.Stats.Potential)))
	if f.Preview != nil {
		f.Preview.DrawBody(f.Env.Screen, f.Left+previewX, f.Top+previewY, previewBattleReady)
	}
	f.gauge(0, int(f.Stats.HP), int(f.Stats.MaxHP))
	f.gauge(1, int(f.Stats.SP), int(f.Stats.MaxSP))
	ratio := float64(0)
	if f.Stats.Formula != nil {
		ratio = f.Stats.Formula.Progress(f.Stats.Level, f.Stats.EXP, f.Stats.Rebirth != 0)
	}
	f.gauge(2, int(ratio*10000), 10000)
	f.label(100, 273, fmt.Sprintf("%d/%d", f.Stats.HP, f.Stats.MaxHP))
	f.label(100, 287, fmt.Sprintf("%d/%d", f.Stats.SP, f.Stats.MaxSP))
	f.label(100, 301, fmt.Sprintf("%.0f%%", ratio*100))
	f.label(45, 318, strconv.Itoa(int(f.Stats.Gold)))
	f.label(160, 317, strconv.Itoa(int(f.Stats.Points)))
	combat := f.Stats.CombatValues()
	for i, v := range combat {
		f.label(44, 337+i*16, strconv.Itoa(int(v)))
	}
	attrs := [5]uint16{f.Stats.STR, f.Stats.CON, f.Stats.INT, f.Stats.WIS, f.Stats.AGI}
	for i, v := range attrs {
		f.label(130, 337+i*16, strconv.Itoa(int(v)))
	}
}
func (f *Form) label(x, y int, s string) {
	f.Env.Text.Draw(f.Left+x, f.Top+y, 0, false, true, f.Env.Screen, []byte(s), 16, 180, inventoryTextPaper, inventoryTextInk, 0)
}

// Gauge bitmaps are FUN_0034fad4's Panel31..36 references. The painter
// FUN_0035172c puts frames at (42,277+14*n), fills at (44,279+14*n).
func (f *Form) gauge(n, v, total int) {
	frame := f.Env.Pics.Find(fmt.Sprintf("Panel%d", 34+n))
	bar := f.Env.Pics.Find(fmt.Sprintf("Panel%d", 31+n))
	f.Env.Pics.Draw(f.Env.Screen, frame, f.Left+42, f.Top+277+14*n, true)
	width, height := f.Env.Pics.Size(bar)
	filled := 0
	if total > 0 {
		filled = int(math.RoundToEven(float64(width) * float64(min(max(v, 0), total)) / float64(total)))
	}
	f.Env.Pics.DrawRect(f.Env.Screen, bar, f.Left+44, f.Top+279+14*n, image.Rect(0, 0, filled, height), true)
}

func (c *slotControl) item() game.Item {
	if c.worn {
		return c.form.State.Equipment[c.slot-1]
	}
	return c.form.State.Bag[c.slot-1]
}
func (c *slotControl) Paint() {
	it := c.item()
	if it.Empty() {
		return
	}
	a := c.Abs()
	c.form.drawItem(it, a.X, a.Y, !c.worn)
}
func (f *Form) drawItem(it game.Item, x, y int, showCount bool) {
	n := f.State.Items[it.ID]
	icon := f.Env.Pics.Find(strconv.Itoa(int(n.Icon)))
	if icon >= 0 {
		f.Env.Pics.Draw(f.Env.Screen, icon, x, y, true)
	}
	if showCount && it.Count > 0 {
		f.Env.Text.Draw(x, y+19, 0, false, true, f.Env.Screen, []byte(strconv.Itoa(int(it.Count))), 13, 32, inventoryTextPaper, inventoryTextInk, 0)
	}
}
func (c *slotControl) LeftDown(shift byte, x, y int) {
	c.Component.LeftDown(shift, x, y)
	f := c.form
	it := c.item()
	if it.Empty() || c.Blocked() || !f.allowed() {
		return
	}
	f.drag = &dragItem{slot: c.slot, worn: c.worn, item: it, start: image.Pt(x, y), split: shift&controlSplit != 0}
	f.Env.UI.Input.Captured = c
}
func (c *slotControl) CapturedMove(shift byte, x, y int) { c.MouseMove(shift, x, y) }
func (c *slotControl) MouseMove(_ byte, x, y int) {
	if d := c.form.drag; d != nil && (abs(x-d.start.X) >= dragThreshold || abs(y-d.start.Y) >= dragThreshold) {
		d.moved = true
	}
}
func (c *slotControl) LeftUp(_ byte, x, y int)        { c.form.release(x, y) }
func (c *slotControl) LeftUpOutside(_ byte, x, y int) { c.form.release(x, y) }
func (c *slotControl) DblClick() {
	c.form.drag = nil
	if !c.Blocked() {
		c.form.Use(c.slot, c.worn)
	}
}
func (c *slotControl) RightUp(_ byte, _, _ int) {
	if !c.Blocked() {
		c.form.Use(c.slot, c.worn)
	}
}
func (f *Form) allowed() bool { return f.Visible && (f.CanAct == nil || f.CanAct()) }
func (f *Form) send(p []byte) {
	if f.allowed() && f.Send != nil {
		f.Send(p)
	}
}
func (f *Form) Use(slot int, worn bool) {
	if !f.allowed() {
		return
	}
	if worn {
		if slot < 1 || slot > EquipmentSlots || f.State.Equipment[slot-1].Empty() {
			return
		}
		free := f.State.FirstFree()
		if free == 0 {
			f.notice("Your inventory is full.")
			return
		}
		f.send([]byte{protocol.CommandInventory, protocol.InventoryUnequip, byte(slot), free})
	} else if slot >= 1 && slot <= game.BagSize && !f.State.Bag[slot-1].Empty() {
		f.send([]byte{protocol.CommandInventory, protocol.InventoryUse, byte(slot)})
	}
}
func (f *Form) release(x, y int) {
	d := f.drag
	f.drag = nil
	if d == nil {
		return
	}
	if abs(x-d.start.X) >= dragThreshold || abs(y-d.start.Y) >= dragThreshold {
		d.moved = true
	}
	if !d.moved || !f.allowed() {
		return
	}
	current := f.State.Bag[d.slot-1]
	if d.worn {
		current = f.State.Equipment[d.slot-1]
	}
	if current != d.item {
		return
	}
	for _, dst := range f.Slots {
		if !dst.Visible || !dst.HitTest(x, y) {
			continue
		}
		if d.worn {
			if dst.item().Empty() {
				f.send([]byte{protocol.CommandInventory, protocol.InventoryUnequip, byte(d.slot), byte(dst.slot)})
			}
			return
		}
		if dst.slot == d.slot {
			return
		}
		request := func(count byte) {
			if f.State.Bag[d.slot-1] == d.item {
				f.send([]byte{protocol.CommandInventory, protocol.InventoryMove, byte(d.slot), count, byte(dst.slot)})
			}
		}
		if d.split && d.item.Count > 1 {
			bag := f.State.Bag
			limit, err := bag.Move(byte(d.slot), byte(dst.slot), d.item.Count, f.State.Items[d.item.ID].Definition.StackLimit())
			if err == nil {
				f.quantity(moveQuantityTitle, limit, request)
			}
		} else {
			request(1)
		}
		return
	}
	for _, dst := range f.Worn {
		if !d.worn && dst.Visible && dst.HitTest(x, y) && int(f.State.equipmentSlot(d.item.ID)) == dst.slot {
			f.send([]byte{protocol.CommandInventory, protocol.InventoryEquip, byte(d.slot)})
			return
		}
	}
	if !image.Pt(x, y).In(f.Rect()) && !d.worn && f.Env.UI.Input.Hovered == nil {
		f.quantity("Drop how many?", d.item.Count, func(count byte) {
			if f.State.Bag[d.slot-1] == d.item {
				f.send([]byte{protocol.CommandInventory, protocol.InventoryDrop, byte(d.slot), count, 0})
			}
		})
	}
}
func (f *Form) DrawDragged() {
	if f.Visible && f.drag != nil && f.drag.moved {
		f.drawItem(f.drag.item, f.Env.UI.Input.X-16, f.Env.UI.Input.Y-16, !f.drag.worn)
	}
}
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func (f *Form) notice(s string) {
	if f.Notice != nil {
		f.Notice(s)
	}
}

// actionDialog uses the existing editor and form framework for quantity and
// drop confirmation. FUN_0035860c supplies the 209×87 form and control coordinates.
// No inventory mutation occurs until a server reply.
type actionDialog struct {
	seui.Form
	owner  *Form
	Count  *seui.Editor
	prompt string
	limit  byte
	submit func(byte)
}

func (f *Form) quantity(prompt string, limit byte, submit func(byte)) {
	if f.Dialog == nil {
		d := &actionDialog{owner: f}
		d.InitForm(d, f.Env)
		d.Init("Form_ThrowThing", (800-209)/2, 0, 0, 0, 0, false, 87, 209, (600-87)/2)
		d.Dockable = false
		d.Count = seui.NewEditor(f.Env, d)
		d.Count.Init("Panel26", 64, 14, 104, 0, 0, true, 15, 81, 33)
		d.Count.SetColor(0x841)
		d.Count.SetAlign(2)
		d.Count.Filter = 1
		d.Count.MaxLen = 2
		d.Count.Limit = true
		b := seui.NewFixedButton(f.Env, d)
		b.Init("Btn_OK_1", 34, 20, 56, 0, 0, true, 20, 56, 59)
		b.OnClick = d.confirm
		b = seui.NewFixedButton(f.Env, d)
		b.Init("Btn_Cancel_1", 119, 20, 56, 0, 0, true, 20, 56, 59)
		b.OnClick = d.Hide
		b = seui.NewFixedButton(f.Env, d)
		b.Init("Btn_ArrowL_5", 37, 17, 17, 0, 0, true, 17, 17, 32)
		b.OnClick = func() { d.adjust(1) }
		b = seui.NewFixedButton(f.Env, d)
		b.Init("Btn_ArrowR_5", 155, 17, 17, 0, 0, true, 17, 17, 32)
		b.OnClick = func() { d.adjust(-1) }
		d.Count.OnEnter = d.confirm
		f.Env.UI.Add(d)
		f.Dialog = d
	}
	d := f.Dialog
	d.prompt, d.limit, d.submit = prompt, limit, submit
	d.Count.SetText([]byte(strconv.Itoa(int(limit))))
	d.Show()
	f.Env.UI.SetModal(true, d)
}
func (d *actionDialog) Paint() {
	d.Env.Pics.Draw(d.Env.Screen, d.Image, d.Left, d.Top, true)
	if d.prompt == moveQuantityTitle {
		return
	}
	// Replace the exported skin's baked "Moving quantity" title for drop and
	// destruction prompts while retaining its native frame and controls.
	d.Env.Screen.Fill(image.Rect(d.Left+32, d.Top+6, d.Right()-2, d.Top+24), 0xffff)
	d.Env.Text.Draw(d.Left+33, d.Top+8, 0, false, false, d.Env.Screen, []byte(d.prompt), 16, 174, 0xffff, 0, 0)
}
func (d *actionDialog) adjust(delta int) {
	n, _ := strconv.Atoi(string(d.Count.Text))
	d.Count.SetText([]byte(strconv.Itoa(min(max(1, n+delta), int(d.limit)))))
}
func (d *actionDialog) Hide() {
	if d.Env.UI.Modal == d {
		d.Env.UI.SetModal(false, d)
	}
	d.submit = nil
	d.Form.Hide()
}
func (d *actionDialog) confirm() {
	n, err := strconv.Atoi(string(d.Count.Text))
	if err != nil || n < 1 || n > int(d.limit) {
		return
	}
	fn := d.submit
	d.Hide()
	if fn != nil {
		fn(byte(n))
	}
}

// ConfirmDestroy requires an explicit confirmation for the server's 23/212
// protected-item response. Stale replies cannot destroy a changed slot.
func (f *Form) ConfirmDestroy(slot byte, id uint16, count byte) {
	if slot < 1 || slot > game.BagSize || count == 0 {
		return
	}
	item := f.State.Bag[slot-1]
	if item.ID != id || item.Count < count {
		return
	}
	if !f.Visible {
		f.Show()
	}
	f.quantity("Destroy how many?", count, func(n byte) {
		if f.State.Bag[slot-1] == item {
			f.send([]byte{protocol.CommandInventory, protocol.InventoryDestroy, slot, n, 0})
		}
	})
}
