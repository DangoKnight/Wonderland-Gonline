package inventory

import (
	"fmt"
	"strconv"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
)

const (
	useDialogWidth     = 299
	useDialogHeight    = 270
	useTargetRowHeight = 30
)

// The target numbering and Potential Pill request follow TPotentialForm
// FUN_001cce98. This compact target selector reuses native controls; the full
// animated PotentialForm presentation remains a separate rendering task.
type useDialog struct {
	seui.Form
	owner   *Form
	slot    byte
	item    game.Item
	target  byte
	Count   *seui.Editor
	Targets [game.MaxPets + 1]*seui.FixedButton
	pets    [game.MaxPets]uint16
}

func (f *Form) openUse(slot byte, item game.Item) {
	if f.UseDialog == nil {
		d := &useDialog{owner: f}
		d.InitForm(d, f.Env)
		d.Init("", (800-useDialogWidth)/2, 0, 0, 0, 0, false, useDialogHeight, useDialogWidth, (600-useDialogHeight)/2)
		d.Dockable = false
		for i := range d.Targets {
			target := byte(i)
			b := seui.NewFixedButton(f.Env, d)
			b.Init("Btn_On_1", 20, 20, 39, 0, 0, true, 20, 39, 45+i*useTargetRowHeight)
			b.OnClick = func() { d.target = target }
			d.Targets[i] = b
		}
		d.Count = seui.NewEditor(f.Env, d)
		d.Count.Init("Panel26", 100, 14, 104, 0, 0, true, 15, 81, 204)
		d.Count.Filter, d.Count.MaxLen, d.Count.Limit = 1, 2, true
		d.Count.SetColor(inventoryTextInk)
		d.Count.OnEnter = d.confirm
		b := seui.NewFixedButton(f.Env, d)
		b.Init("Btn_OK_1", 65, 20, 56, 0, 0, true, 20, 56, 239)
		b.OnClick = d.confirm
		b = seui.NewFixedButton(f.Env, d)
		b.Init("Btn_Cancel_1", 175, 20, 56, 0, 0, true, 20, 56, 239)
		b.OnClick = d.Hide
		f.Env.UI.Add(d)
		f.UseDialog = d
	}
	d := f.UseDialog
	d.slot, d.item, d.target = slot, item, f.Selected
	for i, p := range f.State.Pets {
		d.pets[i] = p.ID
	}
	def := f.State.Items[item.ID].Definition
	if def.Status[0] == itemAmityStatus || def.Status[1] == itemAmityStatus {
		for i, p := range f.State.Pets {
			if p.ID != 0 {
				d.target = byte(i + 1)
				break
			}
		}
	}
	d.Count.SetText([]byte("1"))
	d.Count.Enabled = !potentialPill(item.ID)
	d.syncTargets()
	d.Show()
	f.Env.UI.SetModal(true, d)
}
func (d *useDialog) Hide() {
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
func (d *useDialog) syncTargets() {
	def := d.owner.State.Items[d.item.ID].Definition
	for i, b := range d.Targets {
		b.Enabled = i == 0 || d.owner.State.Pets[i-1].ID != 0
		if i == 0 && (def.Status[0] == itemAmityStatus || def.Status[1] == itemAmityStatus) {
			b.Enabled = false
		}
		name := "Btn_Off_1"
		if byte(i) == d.target {
			name = "Btn_On_1"
		}
		b.Image = d.Env.Pics.Find(name)
	}
}
func (d *useDialog) Paint() {
	d.syncTargets()
	rect := d.Rect()
	d.Env.Screen.Fill(rect, inventoryTextInk)
	d.Env.Screen.Fill(rect.Inset(2), 0xffff)
	text := func(x, y int, s []byte) {
		d.Env.Text.Draw(d.Left+x, d.Top+y, 0, false, true, d.Env.Screen, s, 16, useDialogWidth-x-12, 0xffff, inventoryTextInk, 0)
	}
	title := d.owner.State.Items[d.item.ID].Definition.Name
	if title == "" {
		title = "Use item"
	}
	text(20, 15, clientassets.Big5Text(title))
	for i, b := range d.Targets {
		if !b.Enabled {
			continue
		}
		name := d.owner.PlayerName
		potential := d.owner.Stats.Potential
		if i != 0 {
			name = d.owner.State.Pets[i-1].Name
			potential = d.owner.State.Pets[i-1].Potential
		}
		text(68, 47+i*useTargetRowHeight, name)
		if potentialPill(d.item.ID) {
			text(230, 47+i*useTargetRowHeight, fmt.Appendf(nil, "%d/%d", potential, game.PotentialMaximum))
		}
	}
	text(20, 204, []byte("Quantity"))
	text(194, 204, fmt.Appendf(nil, "/%d", d.item.Count))
	if d.item.ID == game.ItemPotentialPill {
		text(20, 222, []byte("Failure can reduce potential."))
	}
}
func (d *useDialog) confirm() {
	n, err := strconv.Atoi(string(d.Count.Text))
	if err != nil || n < 1 || n > int(d.item.Count) {
		d.owner.notice("Enter a quantity within this stack.")
		return
	}
	if d.owner.State.Bag[d.slot-1] != d.item || (d.target != 0 && d.owner.State.Pets[d.target-1].ID != d.pets[d.target-1]) {
		d.owner.notice("The selected item or pet has changed. Select it again.")
		d.Hide()
		return
	}
	if d.owner.UseSelected(d.slot, byte(n), d.target, d.item) {
		d.Hide()
	}
}
