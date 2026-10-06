package inventory

import (
	"image"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	compoundNativeClass       = "TRe_CompoundForm"
	compoundWidth             = 299
	compoundHeight            = 467
	compoundIngredients       = protocol.CompoundMaximumIngredients
	compoundGridX             = 24
	compoundGridY             = 77
	compoundAnimationFrames   = 23
	compoundAnimationInterval = 200 * time.Millisecond
	compoundReplyTimeout      = 5 * time.Second
	compoundSelectionColor    = 0x8080ff // TColor pink/red selection background.
	compoundSelectionAlpha    = 128
)

// CompoundForm ports TRe_CompoundForm (FUN_0021b240). Ingredient entries
// reference bag anchors; selecting them never moves or consumes inventory.
type CompoundForm struct {
	seui.Form
	Inventory      *Form
	Slots          [game.BagSize]*compoundCell
	Ingredients    [compoundIngredients]*compoundCell
	Synthesis      *seui.FixedButton
	Selection      [compoundIngredients]byte
	selectedItems  [compoundIngredients]game.Item
	CanAct         func() bool
	Send           func([]byte) error
	Notice         func(string)
	Now            func() time.Time
	CanSelect      func(byte) bool
	SkillLearned   func(uint16) bool
	Tier           game.AlchemyTier
	Junior         *seui.FixedButton
	TierButton     *seui.FixedButton
	TierOptions    [3]*seui.FixedButton
	tierMenu       bool
	result         *compoundResult
	visual         *compoundPresentation
	pending        bool
	sentAt         time.Time
	dragSlot       byte
	dragStart      image.Point
	dragMoved      bool
	animationUntil time.Time
}
type compoundCell struct {
	seui.Component
	form       *CompoundForm
	index      int
	ingredient bool
}

func NewCompoundForm(env *seui.Env, inventory *Form) *CompoundForm {
	f := &CompoundForm{Inventory: inventory, Now: time.Now}
	f.InitForm(f, env)
	f.Name = compoundNativeClass
	f.Dockable = false
	f.Init("Form_Compound_1", (800-compoundWidth)/2, 0, 0, 0, 0, false, compoundHeight, compoundWidth, (600-compoundHeight)/2)
	f.initPresentation()
	f.initTierSelector()
	f.button("Btn_Close_s_1", 249, 24, 18, 18, f.Hide)
	f.button("Btn_Close_1", compoundWidth/2+10, 429, 56, 20, f.Hide)
	f.Synthesis = f.button("Btn_Compound_1", compoundWidth/2-66, 429, 56, 20, f.Synthesize)
	for i := range f.Slots {
		f.Slots[i] = f.cell(i, false, compoundGridX+i%BagColumns*cellStep, compoundGridY+i/BagColumns*cellStep, cellSize, cellSize)
	}
	for i := range f.Ingredients {
		y, h := 167, 44
		if i > 0 {
			y, h = 230+(i-1)*51, 32
		}
		f.Ingredients[i] = f.cell(i, true, 206, y, 66, h)
	}
	return f
}
func (f *CompoundForm) button(name string, x, y, w, h int, click func()) *seui.FixedButton {
	b := seui.NewFixedButton(f.Env, f)
	b.Init(name, x, h, w, 0, 0, true, h, w, y)
	b.OnClick = click
	return b
}
func (f *CompoundForm) cell(index int, ingredient bool, x, y, w, h int) *compoundCell {
	c := &compoundCell{form: f, index: index, ingredient: ingredient}
	c.InitComponent(c, f.Env, f)
	c.Init("", x, 0, 0, 0, 0, false, h, w, y)
	c.DownSound = 0xff
	return c
}
func (f *CompoundForm) allowed() bool {
	return f.Visible && !f.pending && !f.Blocked() && (f.CanAct == nil || f.CanAct())
}
func (f *CompoundForm) notice(s string) {
	if f.Notice != nil {
		f.Notice(s)
	}
}
func (f *CompoundForm) Show() { f.Refresh(); f.Form.Show() }
func (f *CompoundForm) Hide() { f.CancelDrag(); f.tierMenu = false; f.result = nil; f.Form.Hide() }
func (f *CompoundForm) Reset() {
	f.Hide()
	f.Selection = [compoundIngredients]byte{}
	f.selectedItems = [compoundIngredients]game.Item{}
	f.pending = false
	f.animationUntil = time.Time{}
	f.Tier = game.AlchemyPrimary
	f.tierMenu = false
	f.result = nil
}
func (f *CompoundForm) CancelDrag() { f.dragSlot = 0; f.dragMoved = false }
func (f *CompoundForm) Refresh() {
	f.advancePresentation()
	f.refreshTierSelector()
	for i, slot := range f.Selection {
		if slot != 0 && (f.Inventory.State.Bag[slot-1].Empty() || f.Inventory.State.Bag[slot-1].Locked || f.CanSelect != nil && !f.CanSelect(slot) || f.Inventory.State.Bag[slot-1].ID != f.selectedItems[i].ID || f.Inventory.State.Bag[slot-1].Damage != f.selectedItems[i].Damage || f.Inventory.State.Bag[slot-1].Metadata != f.selectedItems[i].Metadata) {
			f.Selection[i] = 0
			f.selectedItems[i] = game.Item{}
		}
	}
}
func (f *CompoundForm) SelectIngredient(slot byte, index int) bool {
	if !f.allowed() || slot < 1 || int(slot) > game.BagSize || index < 0 || index >= compoundIngredients {
		return false
	}
	item := f.Inventory.State.Bag[slot-1]
	if item.Empty() || item.Locked || f.CanSelect != nil && !f.CanSelect(slot) {
		f.notice("This item cannot be selected for synthesis.")
		return false
	}
	for _, s := range f.Selection {
		if s == slot {
			return false
		}
	}
	f.Selection[index], f.selectedItems[index] = slot, item
	return true
}
func (f *CompoundForm) AddIngredient(slot byte) {
	for i, s := range f.Selection {
		if s == 0 {
			f.SelectIngredient(slot, i)
			return
		}
	}
	f.notice("All ingredient slots are occupied.")
}
func (f *CompoundForm) Synthesize() {
	if !f.allowed() {
		return
	}
	f.Refresh()
	var slots []byte
	for _, s := range f.Selection {
		if s != 0 {
			slots = append(slots, s)
		}
	}
	materials := 0
	for _, slot := range slots {
		if game.AlchemyBookBonus(f.Inventory.State.Bag[slot-1].ID) == 0 {
			materials++
		}
	}
	if len(slots) < protocol.CompoundIngredientSlots || len(slots) > protocol.CompoundMaximumIngredients || materials < game.AlchemyMinimumMaterials {
		f.notice("Synthesis requires at least two materials; books do not count as materials.")
		return
	}
	if f.Send == nil {
		return
	}
	command := f.AlchemyCommand()
	p := append([]byte{protocol.CommandInventory, command, byte(len(slots))}, slots...)
	if err := f.Send(p); err != nil {
		f.notice(err.Error())
		return
	}
	f.pending, f.sentAt = true, f.Now()
	f.Selection = [compoundIngredients]byte{}
	f.selectedItems = [compoundIngredients]game.Item{}
	f.animationUntil = f.sentAt.Add(compoundAnimationFrames * compoundAnimationInterval)
	f.result = nil
	f.tierMenu = false
}
func (f *CompoundForm) Result(id uint16, count, target byte) {
	if f.result == nil || f.result.slot != target || f.result.item.ID != id || f.result.item.Count != count {
		f.QueueResult(target)
	}
	if f.result != nil && f.result.item.ID == id && f.result.item.Count == count {
		f.result.confirmed = true
		f.result.launch = f.Now()
		if f.result.launch.Before(f.sentAt.Add(compoundResultFrame * compoundAnimationInterval)) {
			f.result.launch = f.sentAt.Add(compoundResultFrame * compoundAnimationInterval)
		}
	}
	f.pending = false
	f.Selection = [compoundIngredients]byte{}
	f.selectedItems = [compoundIngredients]game.Item{}

	name := f.Inventory.State.Items[id].Definition.Name
	if name == "" {
		name = "Item " + strconv.Itoa(int(id))
	}
	f.notice("Synthesis: " + name + " × " + strconv.Itoa(int(count)))
}
func (f *CompoundForm) Paint() {
	if f.tierMenu && f.Env.UI.Input != nil {
		a := f.Abs()
		r := image.Rect(a.X+compoundTierX-3, a.Y+compoundTierMenuY-3, a.X+compoundTierX+compoundTierWidth+3, a.Y+compoundTierY+compoundTierHeight+3)
		if !image.Pt(f.Env.UI.Input.X, f.Env.UI.Input.Y).In(r) {
			f.tierMenu = false
		}
	}
	f.Refresh()
	if f.pending && f.Now().Sub(f.sentAt) >= compoundReplyTimeout {
		f.pending = false
		f.notice("No synthesis result received. Check the ingredients and inventory space.")
	}
	f.Synthesis.Enabled = f.allowed()
	f.Form.Paint()
	if f.tierMenu {
		a := f.Abs()
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Icon_RoleSelectFrame"), a.X+compoundTierX-3, a.Y+compoundTierMenuY-3, true)
	}
}
func (c *compoundCell) item() game.Item {
	if c.ingredient {
		slot := c.form.Selection[c.index]
		if slot == 0 {
			return game.Item{}
		}
		it := c.form.Inventory.State.Bag[slot-1]
		it.Count = 1
		return it
	}
	if c.form.resultHidden(byte(c.index + 1)) {
		return game.Item{}
	}
	return c.form.Inventory.State.Bag[c.index]
}
func (f *CompoundForm) clearIngredient(index int) {
	f.Selection[index] = 0
	f.selectedItems[index] = game.Item{}
}
func (f *CompoundForm) ingredientIndex(slot byte) int {
	for i, selected := range f.Selection {
		if selected == slot {
			return i
		}
	}
	return -1
}
func (c *compoundCell) Paint() {
	it := c.item()
	if it.Empty() {
		return
	}
	a := c.Abs()
	if c.ingredient {
		a.X += (c.Width - cellSize) / 2
		a.Y += (c.Height - cellSize) / 2
	}
	if !c.ingredient && c.form.ingredientIndex(byte(c.index+1)) >= 0 {
		c.Env.Screen.FillAlpha(image.Rect(a.X, a.Y, a.X+cellSize, a.Y+cellSize), compoundSelectionColor, compoundSelectionAlpha)
	}
	c.form.Inventory.drawItem(it, a.X, a.Y, !c.ingredient)
}
func (c *compoundCell) Hint() {
	if !c.item().Empty() && c.form.dragSlot == 0 && !c.Blocked() {
		c.form.Inventory.drawItemHint(&c.Component, c.item())
	}
}
func (c *compoundCell) DblClick() {
	if !c.form.allowed() {
		return
	}
	if c.ingredient {
		c.form.clearIngredient(c.index)
	} else {
		c.form.AddIngredient(byte(c.index + 1))
	}
}
func (c *compoundCell) RightUp(_ byte, _, _ int) {
	if !c.form.allowed() {
		return
	}
	if c.ingredient {
		c.form.clearIngredient(c.index)
	} else if index := c.form.ingredientIndex(byte(c.index + 1)); index >= 0 {
		c.form.clearIngredient(index)
	}
}
func (c *compoundCell) LeftDown(shift byte, x, y int) {
	c.Component.LeftDown(shift, x, y)
	if c.ingredient || c.item().Empty() || !c.form.allowed() {
		return
	}
	c.form.dragSlot = byte(c.index + 1)
	c.form.dragStart = image.Pt(x, y)
	c.form.dragMoved = false
	c.Env.UI.Input.Captured = c
}
func (c *compoundCell) MouseMove(_ byte, x, y int) {
	f := c.form
	if f.dragSlot != 0 && (abs(x-f.dragStart.X) >= dragThreshold || abs(y-f.dragStart.Y) >= dragThreshold) {
		f.dragMoved = true
	}
}
func (c *compoundCell) CapturedMove(s byte, x, y int)  { c.MouseMove(s, x, y) }
func (c *compoundCell) LeftUp(_ byte, x, y int)        { c.release(x, y) }
func (c *compoundCell) LeftUpOutside(_ byte, x, y int) { c.release(x, y) }
func (c *compoundCell) release(x, y int) {
	f := c.form
	if f.dragSlot != 0 && f.dragMoved {
		for i, cell := range f.Ingredients {
			if cell.HitTest(x, y) {
				f.SelectIngredient(f.dragSlot, i)
				break
			}
		}
	} else if f.dragSlot != 0 && c.HitTest(x, y) {
		f.AddIngredient(f.dragSlot)
	}
	f.CancelDrag()
}
func (f *CompoundForm) DrawDragged() {
	if f.Visible && f.dragSlot != 0 && f.dragMoved {
		f.Inventory.drawItem(f.Inventory.State.Bag[f.dragSlot-1], f.Env.UI.Input.X-16, f.Env.UI.Input.Y-16, true)
	}
}
