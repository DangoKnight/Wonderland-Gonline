package inventory

import (
	"fmt"
	"image"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

const (
	itemRemoteControl = 34058
	remoteWidth       = 227
	remoteHeight      = 501
	remoteTabs        = 3
	remotePlusWidth   = 240
	remotePlusHeight  = 489
)

// remoteForm is the main TTH_AutoPlayForm (FUN_001e5ba4). Its settings
// window is THL_AutoPlayFormPlus. These independent, nonmodal forms do not
// consume the controller. Confirm starts the client automation controller.
type remoteForm struct {
	seui.Form
	owner                 *Form
	Plus                  *remotePlusForm
	Settings, Start, Stop *seui.FixedButton
	Checks                [11]*seui.FixedButton
	Thresholds            [4]*seui.ScrollButton
	checked               [11]bool
	Fields                [5]*seui.Editor
	Discard               [5]uint16
	DiscardCells          [5]*seui.Component
}

type remotePlusForm struct {
	seui.Form
	tab  int
	Tabs [remoteTabs]*seui.FixedButton
}

func remoteButton(env *seui.Env, owner seui.Control, asset string, x, y, w, h int, click func()) *seui.FixedButton {
	b := seui.NewFixedButton(env, owner)
	b.Init(asset, x, h, w, 0, 0, true, h, w, y)
	b.OnClick = click
	return b
}

func (f *Form) openRemote(slot byte) {
	if f.State.Bag[slot-1].ID != itemRemoteControl {
		return
	}
	if f.Remote == nil {
		d := &remoteForm{owner: f}
		d.InitForm(d, f.Env)
		d.Init("form_autoPlay_1", (800-remoteWidth)/2, 0, 0, 0, 0, false, remoteHeight, remoteWidth, (600-remoteHeight)/2)
		d.Dockable = false
		remoteButton(f.Env, d, "Btn_Close_s_1", 152, 24, 18, 18, d.Hide).Hide()
		remoteButton(f.Env, d, "Btn_Close_2", 132, 463, 39, 20, d.Hide)
		d.Start = remoteButton(f.Env, d, "Btn_OK_2", 34, 463, 39, 20, d.start)
		d.Start.SetHint([]byte("Apply Remote settings and start automation"))
		d.Stop = remoteButton(f.Env, d, "Btn_Stop_2", 83, 463, 39, 20, d.stop)
		d.Settings = remoteButton(f.Env, d, "Btn_PlusSet", 115, 46, 61, 20, d.openSettings)
		// Native checkbox order: fight, move, supply, no supplies, player
		// deaths, pet deaths, elapsed time, unequip, discard, compatibility
		// option 10 (hidden), remote information.
		points := [11][2]int{{19, 48}, {19, 70}, {19, 92}, {39, 267}, {19, 289}, {19, 311}, {19, 333}, {19, 355}, {19, 377}, {19, 399}, {19, 438}}
		for i, point := range points {
			index := i
			d.Checks[i] = remoteButton(f.Env, d, "btn_UnCheck_1", point[0], point[1], 16, 16, func() {
				d.checked[index] = !d.checked[index]
				d.refreshCheck(index)
			})
		}
		d.Checks[9].SetVisible(false)
		d.checked[10] = true
		d.refreshCheck(10)
		editor := func(index, x, y, w int, value string) {
			e := seui.NewEditor(f.Env, d)
			e.Init("", x, 0, 0, 0, 0, false, 18, w, y)
			e.Filter, e.MaxLen, e.Limit = 1, 4, true
			e.Color = inventoryTextInk
			e.SetText([]byte(value))
			d.Fields[index] = e
		}
		editor(0, 93, 114, 30, "1")
		editor(1, 140, 114, 30, "50")
		editor(2, 95, 289, 30, "3")
		editor(3, 95, 311, 30, "3")
		editor(4, 55, 333, 40, "0")
		for i, y := range []int{161, 183, 227, 249} {
			s := seui.NewScrollButton(f.Env, d)
			s.Init("", 106, 6, 70, 0, 0, true, 10, 70, y)
			s.SetThumb("bar_w4", 0, 12, 15, 0)
			s.SetCaps(7, 12)
			s.SetRange(remotePercentSteps, 1)
			s.SetVertical(false)
			s.SetPos(remoteDefaultThreshold)
			d.Thresholds[i] = s
		}

		for i := range d.DiscardCells {
			index := i
			cell := seui.NewComponent(f.Env, d)
			cell.Init("", 20+i*34, 0, 0, 0, 0, false, 30, 30, 399)
			cell.OnUp = func() { d.Discard[index] = 0 }
			d.DiscardCells[i] = cell
		}
		f.Env.UI.Add(d)
		f.Remote = d
	}
	// Only opening Remote closes inventory. Either window may subsequently
	// be opened, dragged and closed without changing the other's visibility.
	f.Hide()
	f.Remote.Show()
}

const (
	remotePercentSteps     = 101
	remoteDefaultThreshold = 50
	remoteEscapeKey        = 0x1b
)

func (d *remoteForm) refreshCheck(i int) {
	name := "btn_UnCheck_1"
	if d.checked[i] {
		name = "btn_Check_1"
	}
	d.Checks[i].Image = d.Env.Pics.Find(name)
}
func (d *remoteForm) Paint() {
	d.Env.Pics.Draw(d.Env.Screen, d.Image, d.Left, d.Top, true)
	for i, id := range d.Discard {
		if id != 0 {
			d.owner.drawItem(game.Item{ID: id, Count: 1}, d.Left+20+i*34, d.Top+399, false)
		}
	}
	for i, y := range []int{157, 179, 223, 245} {
		d.Env.Screen.Fill(image.Rect(d.Left+106, d.Top+y+6, d.Left+176, d.Top+y+12), inventoryTextInk)
		d.Env.Text.Draw(d.Left+181, d.Top+y, 0, false, true, d.Env.Screen, fmt.Appendf(nil, "%d%%", d.Thresholds[i].Pos), 16, 45, inventoryTextPaper, inventoryTextInk, 0)
	}
}
func (d *remoteForm) KeyDown(key uint16, _ byte) {
	if key == remoteEscapeKey {
		d.Hide()
	}
}
func (d *remoteForm) openSettings() {
	if d.Plus == nil {
		p := &remotePlusForm{}
		p.InitForm(p, d.Env)
		p.Init("Form_AutoPlayerPlus_1", min(d.Left+remoteWidth+36, 800-remotePlusWidth), 0, 0, 0, 0, false, remotePlusHeight, remotePlusWidth, d.Top)
		p.Dockable = false
		remoteButton(d.Env, p, "Btn_Close_s_1", 152, 24, 18, 18, p.Hide).Hide()
		remoteButton(d.Env, p, "Btn_Close_2", 149, 453, 39, 20, p.Hide)
		for i := range p.Tabs {
			tab := i
			p.Tabs[i] = remoteButton(d.Env, p, fmt.Sprintf("Icon_AutoTab_%d", i+1), 19+i*66, 41, 66, 21, func() { p.selectTab(tab) })
		}
		remoteButton(d.Env, p, "Btn_OK_2", 51, 453, 39, 20, func() {
			d.owner.notice("Advanced skill assignment is not implemented yet. Auto Atk uses basic attacks.")
		})
		d.Env.UI.Add(p)
		d.Plus = p
	}
	d.Plus.selectTab(0)
	d.Plus.Show()
}

// ResetRemote is a character/session boundary, unlike closing Inventory.
func (f *Form) ResetRemote() {
	if f.StopRemote != nil {
		f.StopRemote()
	}
	if f.Remote == nil {
		return
	}
	f.Remote.Hide()
	if f.Remote.Plus != nil {
		f.Remote.Plus.Hide()
	}
}
func (d *remotePlusForm) selectTab(tab int) {
	d.tab = tab
	skin := "Form_AutoPlayerPlus_1"
	if tab == remoteTabs-1 {
		skin = "Form_AutoPlayerPlus_2"
	}
	d.Image = d.Env.Pics.Find(skin)
	for i, b := range d.Tabs {
		b.Sticky = i == tab
	}
}
func (d *remotePlusForm) KeyDown(key uint16, _ byte) {
	if key == remoteEscapeKey {
		d.Hide()
	}
}

// OpenRemote reopens the controller from its on-screen shortcut.
func (f *Form) OpenRemote(slot byte) {
	if slot >= 1 && int(slot) <= len(f.State.Bag) {
		f.openRemote(slot)
	}
}
