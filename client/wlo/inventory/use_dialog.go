package inventory

import (
	"fmt"
	"image"
	"time"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
)

// TPotentialForm: FUN_001cac48, FUN_001cb6f0, FUN_001cc3ac, FUN_001cd574.
const (
	useDialogWidth              = 299
	useDialogHeight             = 520
	potentialRowLeft            = 32
	potentialRowTop             = 79
	potentialRowWidth           = 233
	potentialRowHeight          = 34
	potentialAttributeLeft      = 60
	potentialAttributeTop       = 274
	potentialAttributeStep      = 16
	potentialBonusLeft          = 86
	potentialPreviewX           = 193
	potentialPreviewY           = 349
	potentialPillX              = 56
	potentialPillY              = 366
	potentialStackDisplayLimit  = 999
	potentialSelectionColor     = 0xf5bf89
	potentialSelectionAlpha     = 80
	potentialFlameFrameInterval = 50 * time.Millisecond
	potentialFlameFrames        = 4
	potentialEarnedFlameHeight  = 26
	potentialNextFlameHeight    = 14
)

// Native coordinates are the mark's upper-left corners, clockwise from top right.
var potentialMarks = [...]image.Point{{201, 259}, {230, 277}, {245, 305}, {245, 337}, {230, 367}, {201, 385}, {167, 385}, {138, 367}, {123, 337}, {123, 305}, {138, 277}, {167, 259}}

type useDialog struct {
	seui.Form
	owner                  *Form
	slot                   byte
	item                   game.Item
	target                 byte
	selected               bool
	petID                  uint16
	base                   [5]uint16
	Targets                [game.MaxPets + 1]*potentialRow
	Confirm, Cancel, Close *seui.FixedButton
	opened                 time.Time
	warning                *potentialWarning
}

type potentialRow struct {
	seui.Component
	dialog *useDialog
	target byte
}

func (r *potentialRow) Paint() {}
func (r *potentialRow) LeftUp(_ byte, _, _ int) {
	if !r.Blocked() {
		r.dialog.SelectTarget(r.target)
	}
}

func (f *Form) openUse(slot byte, item game.Item) {
	if f.LoadPotentialPictures != nil {
		f.LoadPotentialPictures()
	}
	if f.UseDialog == nil {
		d := &useDialog{owner: f}
		d.InitForm(d, f.Env)
		d.Name = "TPotentialForm"
		d.Dockable = false
		d.Init("Form_Potential_1", (800-useDialogWidth)/2, 0, 0, 0, 0, false, useDialogHeight, useDialogWidth, (600-useDialogHeight)/2)
		button := func(name string, x, y, w, h int, click func()) *seui.FixedButton {
			b := seui.NewFixedButton(f.Env, d)
			b.Init(name, x, h, w, 0, 0, true, h, w, y)
			b.OnClick = click
			return b
		}
		d.Close = button("Btn_Close_s_1", 249, 26, 18, 18, d.Hide)
		d.Cancel = button("Btn_Cancel_1", 163, 473, 56, 20, d.Hide)
		d.Cancel.SetHint([]byte("[ESC]"))
		d.Confirm = button("Btn_confirm_1", 87, 473, 56, 20, d.confirm)
		for i := range d.Targets {
			r := &potentialRow{dialog: d}
			r.InitComponent(r, f.Env, d)
			r.Init("", potentialRowLeft, 0, 0, 0, 0, false, potentialRowHeight, potentialRowWidth, potentialRowTop+i*potentialRowHeight)
			r.DownSound = 0xff
			d.Targets[i] = r
		}
		f.Env.UI.Add(d)
		f.UseDialog = d
	}
	d := f.UseDialog
	d.slot, d.item, d.target, d.selected, d.petID = slot, item, 0, false, 0
	d.opened = f.itemUseTime()
	d.syncTargets()
	d.Show()
	f.Env.UI.SetModal(true, d)
}

func (d *useDialog) Hide() {
	if d.warning != nil {
		d.warning.Hide()
	}
	if d.Env.UI.Modal == d {
		d.Env.UI.SetModal(false, d)
	}
	d.Form.Hide()
}

func (d *useDialog) KeyDown(key uint16, _ byte) {
	if key == 0x1b {
		d.Hide()
	}
}

func (d *useDialog) stats(target byte) *world.Stats {
	if target == 0 {
		return d.owner.Stats
	}
	return &d.owner.State.Pets[target-1].Stats
}

func (d *useDialog) potential(target byte) byte {
	if target == 0 {
		return d.owner.Stats.Potential
	}
	return d.owner.State.Pets[target-1].Potential
}

func (d *useDialog) SelectTarget(target byte) {
	if d.warning != nil || target > game.MaxPets || target != 0 && d.owner.State.Pets[target-1].ID == 0 || d.owner.potentialRequest != nil {
		return
	}
	d.target, d.selected, d.petID = target, true, 0
	if target != 0 {
		d.petID = d.owner.State.Pets[target-1].ID
	}
	s := d.stats(target)
	attributes := [5]uint16{s.STR, s.CON, s.INT, s.WIS, s.AGI}
	bonus := game.PotentialBonus(uint16(d.potential(target)))
	for i, value := range attributes {
		d.base[i] = uint16(max(0, int(value)-int(bonus)))
	}
	d.syncTargets()
}

func (d *useDialog) pillCount() int {
	total := 0
	for _, item := range d.owner.State.Bag {
		if item.ID == d.item.ID && !item.Empty() && !item.Locked {
			total += int(item.Count)
		}
	}
	return min(total, potentialStackDisplayLimit)
}

func (d *useDialog) syncTargets() {
	if d.selected && d.target != 0 && d.owner.State.Pets[d.target-1].ID != d.petID {
		d.selected = false
	}
	n := 1
	d.Targets[0].target, d.Targets[0].Visible = 0, true
	for i, pet := range d.owner.State.Pets {
		if pet.ID == 0 {
			continue
		}
		d.Targets[n].target, d.Targets[n].Visible = byte(i+1), true
		n++
	}
	for ; n < len(d.Targets); n++ {
		d.Targets[n].Visible = false
	}
	d.Confirm.Visible = d.selected && d.pillCount() > 0 && d.owner.potentialRequest == nil
	d.Confirm.Enabled = d.Confirm.Visible
}

func (d *useDialog) text(x, y int, s string, ink uint16) {
	d.Env.Text.Draw(d.Left+x, d.Top+y, 0, false, true, d.Env.Screen, []byte(s), 16, useDialogWidth-x, 0, ink, 0)
}

func (d *useDialog) Paint() {
	d.syncTargets()
	d.Env.Pics.Draw(d.Env.Screen, d.Image, d.Left, d.Top, true)
	for _, row := range d.Targets {
		if !row.Visible {
			continue
		}
		a := row.Abs()
		if d.selected && row.target == d.target {
			d.Env.Screen.FillAlpha(row.Rect().Inset(1), potentialSelectionColor, potentialSelectionAlpha)
		}
		if d.owner.DrawPotentialPortrait != nil {
			d.owner.DrawPotentialPortrait(row.target, image.Rect(a.X, a.Y, a.X+32, a.Y+32))
		}
		name := d.owner.PlayerName
		if row.target != 0 {
			name = d.owner.State.Pets[row.target-1].Name
		}
		d.Env.Text.Draw(a.X+38, a.Y+8, 0, false, true, d.Env.Screen, name, 16, 110, 0, inventoryTextInk, 0)
		level := fmt.Sprint(d.stats(row.target).Level)
		d.text(potentialRowLeft+38+127-len(level)*8, row.Top+8, level, inventoryTextInk)
		maxText := fmt.Sprintf("/%d", game.PotentialMaximum)
		value := fmt.Sprint(d.potential(row.target))
		d.text(potentialRowLeft+38+190-len(maxText)*8, row.Top+8, maxText, inventoryTextInk)
		d.text(potentialRowLeft+38+190-(len(maxText)+len(value))*8, row.Top+8, value, 0xf800)
	}
	if !d.Confirm.Visible {
		d.Env.Pics.Draw(d.Env.Screen, d.Env.Pics.Find("icon_Confirm_1"), d.Left+87, d.Top+473, true)
	}
	pill := d.item
	pill.Count = 0
	d.owner.drawItem(pill, d.Left+potentialPillX, d.Top+potentialPillY, false)
	d.text(potentialPillX, potentialPillY+19, fmt.Sprint(d.pillCount()), inventoryTextInk)
	if !d.selected {
		return
	}
	bonus := game.PotentialBonus(uint16(d.potential(d.target)))
	for i, value := range d.base {
		d.text(potentialAttributeLeft, potentialAttributeTop+i*potentialAttributeStep, fmt.Sprint(value), inventoryTextInk)
		d.text(potentialBonusLeft, potentialAttributeTop+i*potentialAttributeStep, fmt.Sprintf("+%d", bonus), 0xf800)
	}
	if d.owner.DrawPotentialPreview != nil {
		d.owner.DrawPotentialPreview(d.target, d.Left+potentialPreviewX, d.Top+potentialPreviewY)
	}
	d.drawMarks()
}

func (d *useDialog) drawMarks() {
	level := min(int(d.potential(d.target)), game.PotentialMaximum)
	frame := int(d.owner.itemUseTime().Sub(d.opened)/potentialFlameFrameInterval) % potentialFlameFrames
	for i := 0; i < min(level+1, len(potentialMarks)); i++ {
		at := potentialMarks[i].Add(image.Pt(d.Left, d.Top))
		d.Env.Pics.Draw(d.Env.Screen, d.Env.Pics.Find("Icon_PotentialMark_1"), at.X, at.Y, true)
		name, height, dx, dy := "PotentialFire1", potentialEarnedFlameHeight, 2, -11
		if i == level {
			name, height, dx, dy = "PotentialFire2", potentialNextFlameHeight, 7, 1
		}
		pic := d.Env.Pics.Find(name)
		w, _ := d.Env.Pics.Size(pic)
		d.Env.Pics.DrawRect(d.Env.Screen, pic, at.X+dx, at.Y+dy, image.Rect(0, frame*height, w, (frame+1)*height), true)
	}
}

func (d *useDialog) confirm() {
	if d.warning != nil {
		return
	}
	if d.selected && d.target == 0 && d.item.ID == game.ItemPotentialPill && d.owner.PotentialSkillsAtRisk != nil {
		chance, err := game.PotentialSuccessPercent(d.item.ID, uint16(d.potential(0)))
		if err == nil && chance < game.PotentialChanceScale {
			if names := d.owner.PotentialSkillsAtRisk(); len(names) > 0 {
				d.showSkillWarning(names)
				return
			}
		}
	}
	d.submit()
}

func (d *useDialog) submit() {
	if !d.selected || d.owner.potentialRequest != nil {
		return
	}
	if d.target != 0 && d.owner.State.Pets[d.target-1].ID != d.petID {
		d.selected = false
		d.syncTargets()
		return
	}
	// Native scans all matching stacks each attempt; the request still consumes
	// exactly one pill from its current, authoritative anchor slot.
	for i, item := range d.owner.State.Bag {
		if item.ID != d.item.ID || item.Empty() || item.Locked {
			continue
		}
		d.owner.UseSelected(byte(i+1), 1, d.target, item)
		d.syncTargets()
		return
	}
	d.syncTargets()
}
