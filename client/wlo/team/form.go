package team

import (
	"bytes"
	"fmt"
	"image"
	"strconv"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	formWidth                  = 356
	formHeight                 = 539
	petRowTop                  = 77
	petRowStep                 = 34
	memberRowTop               = 265
	memberRowStep              = 75
	textInk                    = 0x0841
	petPortraitTop             = 69 // First cell interior in Form_Team_2, above the action buttons.
	petIconSize                = 24
	petIconInset               = (petRowStep - petIconSize) / 2
	petNameLimit               = 14 // Native TSe_Editor limit in FUN_0026d6bc.
	PetConfirmationAction      = 11 // FUN_0034c814: NPC body mode, fixed first frame.
	confirmationBodyMargin     = 30
	confirmationBodyTop        = 20
	confirmationNameGap        = 16
	confirmationBodyPadding    = 70
	confirmationButtonRow      = 40
	confirmationButtonsUp      = 45
	confirmationLowBodyHeight  = 86
	confirmationLowNamePadding = 68
	modeRest                   = 0
	modeBattle                 = 1
	modeRide                   = 2
)

type Form struct {
	Instances *Instances
	Tabs      [2]*seui.FixedButton
	seui.Form
	State              *State
	Inventory          *inventory.Form
	CanAct             func() bool
	Send               func([]byte) error
	Notice             func(string)
	NameOf             func(uint32) []byte
	StatsOf            func(uint32) *world.Stats
	DrawFace           func(uint32, image.Rectangle)
	DrawPetFace        func(byte, image.Rectangle)
	DrawPetBody        func(byte, int, int, int)
	PetPreviewSize     func(byte) (int, int)
	PetPreviewTemplate func(byte) world.NPCTemplate
	Modes              [game.MaxPets]*seui.FixedButton
	Dismiss            [game.MaxPets]*seui.FixedButton
	Names              [game.MaxPets]*seui.Editor
	Kick               [MaximumMembers - 1]*seui.FixedButton
	Leader             [MaximumMembers - 1]*seui.FixedButton
	End                *seui.FixedButton
	Options            [3]*seui.FixedButton
	editingSlot        byte
	editingID          uint16
	menuSlot           byte
	prompt             *confirmation
	status             *login.Status
}

// FUN_0026d6bc/FUN_002756c4: four companion rows above three teammate rows.
func NewForm(env *seui.Env, state *State, bag *inventory.Form) *Form {
	f := &Form{State: state, Inventory: bag, status: login.NewStatus(env, nil)}
	f.InitForm(f, env)
	f.Name = "TSe_TeamForm"
	f.Dockable = false
	f.Init("Form_Team_2", 168, 0, 0, 0, 0, false, formHeight, formWidth, 20)
	for mode, asset := range []string{"Btn_AssignRest_1", "Btn_AssignFight_1", "Btn_AssignRide_1"} {
		choice := mode
		f.Options[mode] = f.button(asset, 247, 0, 44, 20, func() { f.SetMode(f.menuSlot, choice) })
		f.Options[mode].Visible = false
		f.Options[mode].SetHint([]byte([]string{"Rest the pet.", "Assign the pet to battle.", "Ride the pet."}[mode]))
	}
	f.button("Btn_Close_s_1", 321, 24, 18, 18, f.Hide)
	f.button("Btn_Close_1", 157, 500, 56, 20, f.Hide)
	f.End = f.button("Btn_DismissTeam_1", 140, 235, 70, 20, func() {
		f.Confirm("Leave the party?", 0, func() { f.send([]byte{protocol.CommandTeam, protocol.TeamLeave}) })
	})
	for i := range f.Modes {
		slot := byte(i + 1)
		e := seui.NewEditor(env, f)
		e.Init("", 63, 13, 83, 0, 0, true, 23, 86, 73+i*petRowStep)
		e.ReadOnly = true
		e.PixelHit = false // The idle name has no panel image but remains clickable.
		e.Color = textInk
		e.SetMaxLen(petNameLimit)
		e.SetFilter(3)
		e.SetHint([]byte("Change name"))
		e.OnClick = func() { f.editName(slot) }
		e.OnEnter = f.finishName
		e.OnBlur = f.finishName
		f.Names[i] = e
		f.Modes[i] = f.button("Btn_AssignRest_1", 247, petRowTop+i*petRowStep, 44, 20, func() {
			if f.menuSlot == slot {
				f.menuSlot = 0
			} else {
				f.menuSlot = slot
			}
			f.Refresh()
		})
		f.Dismiss[i] = f.button("Btn_AssignLeave_1", 295, petRowTop+i*petRowStep, 44, 20, func() { f.DismissPet(slot) })
	}
	for i := range f.Kick {
		row := i
		f.Leader[i] = f.button("Btn_AssignLeader_1", 265, memberRowTop+i*memberRowStep+17, 70, 20, func() { f.memberAction(row, true) })
		f.Kick[i] = f.button("Btn_AssignLeave_1", 265, memberRowTop+i*memberRowStep+43, 44, 20, func() { f.memberAction(row, false) })
	}
	f.Instances = NewInstances(f)
	f.Tabs[0] = f.button("Btn_Team_1", 20, 41, 66, 21, func() { f.SelectTab(false) })
	f.Tabs[1] = f.button("Btn_DupMis", 85, 41, 66, 21, f.Instances.Show)
	f.SelectTab(false)
	return f
}
func (f *Form) button(asset string, x, y, w, h int, click func()) *seui.FixedButton {
	b := seui.NewFixedButton(f.Env, f)
	b.Init(asset, x, h, w, 0, 0, true, h, w, y)
	b.OnClick = click
	return b
}
func (f *Form) allowed() bool { return f.Visible && !f.Blocked() && (f.CanAct == nil || f.CanAct()) }
func (f *Form) send(p []byte) {
	if f.Send != nil {
		if err := f.Send(p); err != nil && f.Notice != nil {
			f.Notice(err.Error())
		}
	}
}
func (f *Form) Show() { f.Form.Show(); f.Refresh() }
func (f *Form) SelectTab(instances bool) {
	f.finishName()
	f.menuSlot = 0
	f.Instances.Visible = instances
	if !instances {
		f.Instances.New.Hide()
	}
	f.Refresh()
}

// PointerDown is called before dispatch so a world click also dismisses the menu.
func (f *Form) PointerDown(x, y int) {
	if f.menuSlot == 0 {
		return
	}
	for _, option := range f.Options {
		if option.Visible && option.HitTest(x, y) {
			return
		}
	}
	if f.Modes[f.menuSlot-1].HitTest(x, y) {
		return
	}
	f.menuSlot = 0
	f.Refresh()
}
func (f *Form) Hide() {
	f.finishName()
	f.menuSlot = 0
	if f.prompt != nil {
		f.prompt.Hide()
	}
	if f.Instances != nil {
		f.Instances.New.Hide()
	}
	f.Form.Hide()
}
func (f *Form) Reset() { f.Hide(); f.Instances.Reset(); f.State.Reset(0) }

// FUN_00272004/FUN_00271eac: click the name to edit; Enter or blur submits.
func (f *Form) editName(slot byte) {
	if !f.allowed() || f.Instances.Visible || slot < 1 || slot > game.MaxPets {
		return
	}
	if f.editingSlot == slot {
		return
	}
	f.finishName()
	pet := f.Inventory.State.Pets[slot-1]
	if pet.ID == 0 {
		return
	}
	f.editingSlot, f.editingID = slot, pet.ID
	e := f.Names[slot-1]
	e.ReadOnly = false
	e.Image = f.Env.Pics.Find("Panel29")
	e.Color = 0xffff
	e.SetText(pet.Name)
	e.Focused = true
	f.Env.UI.Input.Focused = e
}
func (f *Form) finishName() {
	if f.editingSlot == 0 {
		return
	}
	slot, id := f.editingSlot, f.editingID
	f.editingSlot, f.editingID = 0, 0
	e := f.Names[slot-1]
	name := append([]byte(nil), e.Text...)
	e.ReadOnly, e.Focused, e.Image, e.Color = true, false, -1, textInk
	if f.Env.UI.Input.Focused == e {
		f.Env.UI.Input.Focused = nil
	}
	pet := f.Inventory.State.Pets[slot-1]
	if f.allowed() && pet.ID == id && len(name) > 0 && len(name) <= petNameLimit && !bytes.Equal(name, pet.Name) {
		f.send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlRename, slot}.Bytes(name))
	}
	e.SetText(pet.Name)
	e.Caret, e.CaretCol = 0, 0
}

// FUN_0027758c/FUN_00277fc8: mode changes are requests, not optimistic roster edits.
func (f *Form) SetMode(slot byte, mode int) {
	if !f.allowed() || slot < 1 || slot > game.MaxPets {
		return
	}
	pet := f.Inventory.State.Pets[slot-1]
	if pet.ID == 0 {
		return
	}
	f.menuSlot = 0
	if mode != modeRest && pet.Stats.HP == 0 {
		if f.Notice != nil {
			f.Notice("Can't use a defeated pet.")
		}
		return
	}
	if mode == modeBattle && pet.Amity < 40 {
		if f.Notice != nil {
			f.Notice("Can't use, Amity below 40")
		}
		return
	}
	if f.State.MountPet == pet.ID && mode != modeRide {
		f.send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlUnmountPet, slot}.U32(uint32(pet.ID)))
	}
	if f.State.BattlePet == pet.ID && mode != modeBattle {
		f.send([]byte{protocol.CommandBattlePet, protocol.BattlePetRest})
	}
	switch mode {
	case modeBattle:
		f.send(protocol.Builder{protocol.CommandBattlePet, protocol.BattlePetSelect}.U32(uint32(pet.ID)))
	case modeRide:
		f.send(protocol.Builder{protocol.CommandPetControl, protocol.PetControlMountPet, slot}.U32(uint32(pet.ID)))
	}
	f.Refresh()
}

// FUN_00272440: permanent release requires explicit confirmation and stable slot identity.
func (f *Form) DismissPet(slot byte) {
	if !f.allowed() || slot < 1 || slot > game.MaxPets {
		return
	}
	pet := f.Inventory.State.Pets[slot-1]
	if pet.ID == 0 {
		return
	}
	f.Hide()
	f.Confirm("Remove "+string(pet.Name)+"?", slot, func() {
		if f.CanAct != nil && !f.CanAct() {
			return
		}
		if f.Inventory.State.Pets[slot-1].ID == pet.ID {
			f.send([]byte{protocol.CommandPetControl, protocol.PetControlPetSlot, slot})
		}
	})
}
func (f *Form) memberAction(row int, leader bool) {
	ids := f.State.Others()
	if !f.allowed() || f.State.Leader != f.State.Self || row >= len(ids) {
		return
	}
	id := ids[row]
	message := "Dismiss this teammate?"
	code := byte(protocol.TeamKick)
	if leader {
		message = "Appoint this teammate as leader?"
		code = protocol.TeamTransferLeadership
	}
	f.Confirm(message, 0, func() {
		if f.State.Leader != f.State.Self || f.CanAct != nil && !f.CanAct() {
			return
		}
		for _, member := range f.State.Others() {
			if member == id {
				f.send(protocol.Builder{protocol.CommandTeam, code}.U32(id))
				return
			}
		}
	})
}
func (f *Form) Refresh() {
	can := f.allowed()
	// FUN_0026dfc4 keeps the active tab pressed, like the Skills tabs.
	for i, b := range f.Tabs {
		b.Sticky = (i == 1) == f.Instances.Visible
		if b.Sticky {
			b.State = 2
		} else {
			b.State = 0
		}
	}
	for i, pet := range f.Inventory.State.Pets {
		have := pet.ID != 0 && !f.Instances.Visible
		e := f.Names[i]
		e.Visible, e.Enabled = have, can
		if f.editingSlot == byte(i+1) && (!have || pet.ID != f.editingID) {
			// The old editor must never rename a replacement occupying this slot.
			f.editingID = 0
			f.finishName()
		}
		if f.editingSlot != byte(i+1) && !bytes.Equal(e.Text, pet.Name) {
			e.SetText(pet.Name)
			e.Caret, e.CaretCol = 0, 0
		}
		f.Modes[i].Visible = have
		f.Modes[i].Enabled = can
		f.Dismiss[i].Visible = have
		f.Dismiss[i].Enabled = can
		art := "Btn_AssignRest_1"
		if f.State.BattlePet == pet.ID && have {
			art = "Btn_AssignFight_1"
		}
		if f.State.MountPet == pet.ID && have {
			art = "Btn_AssignRide_1"
		}
		f.Modes[i].Image = f.Env.Pics.Find(art)
	}
	if f.menuSlot != 0 && f.Inventory.State.Pets[f.menuSlot-1].ID == 0 {
		f.menuSlot = 0
	}
	for i, b := range f.Options {
		b.Visible = f.menuSlot != 0 && !f.Instances.Visible
		b.Enabled = can
		b.Top = petRowTop + (int(f.menuSlot)-1)*petRowStep - 63 + i*20
	}

	ids := f.State.Others()
	f.End.Visible = f.State.InParty() && !f.Instances.Visible
	f.End.Enabled = can
	art := "Btn_LeaveTeam_1"
	if f.State.Leader == f.State.Self {
		art = "Btn_DismissTeam_1"
	}
	f.End.Image = f.Env.Pics.Find(art)
	for i := range f.Kick {
		show := i < len(ids) && f.State.Leader == f.State.Self && !f.Instances.Visible
		f.Kick[i].Visible = show
		f.Leader[i].Visible = show
		f.Kick[i].Enabled = can
		f.Leader[i].Enabled = can
	}
}
func (f *Form) Paint() {
	f.Refresh()
	art := "Form_Team_2"
	if f.Instances.Visible {
		art = "Form_Emblem_1"
	}
	f.Image = f.Env.Pics.Find(art)
	width, height := f.Env.Pics.Size(f.Image)
	f.Clip = image.Rect(0, 0, width, height)
	f.Form.Paint()
	if f.Instances.Visible {
		return
	}
	a := f.Abs()

	text := func(x, y int, s []byte) {
		f.Env.Text.Draw(a.X+x, a.Y+y, 0, false, true, f.Env.Screen, s, 15, formWidth-x-10, 0, textInk, 0)
	}
	for i, pet := range f.Inventory.State.Pets {
		if pet.ID == 0 {
			continue
		}
		y := petRowTop + i*petRowStep
		if f.DrawPetFace != nil {
			f.DrawPetFace(byte(i+1), image.Rect(a.X+24+petIconInset, a.Y+petPortraitTop+i*petRowStep+petIconInset, a.X+24+petIconInset+petIconSize, a.Y+petPortraitTop+i*petRowStep+petIconInset+petIconSize))
		}
		f.smallLevel(a.X+166, a.Y+y, pet.Stats.Level)
		f.smallVitals(a.X+166, a.Y+y+10, &pet.Stats)
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(fmt.Sprintf("Icon_Element_%d_1", pet.Stats.Element)), a.X+213, a.Y+y, true)
	}
	for i, id := range f.State.Others() {
		if i >= len(f.Kick) {
			break
		}
		y := memberRowTop + i*memberRowStep
		if f.DrawFace != nil {
			f.DrawFace(id, image.Rect(a.X+25, a.Y+y-3, a.X+97, a.Y+y+69))
		}
		name := []byte("Player " + strconv.FormatUint(uint64(id), 10))
		if f.NameOf != nil && len(f.NameOf(id)) > 0 {
			name = f.NameOf(id)
		}
		text(105, y+4, name)
		var stats *world.Stats
		if f.StatsOf != nil {
			stats = f.StatsOf(id)
		}
		if stats != nil {
			f.status.Level(stats.Level, image.Pt(a.X+200, a.Y+y+18), true)
			f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Word_Hp_2"), a.X+105, a.Y+y+18, true)
			f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Word_Sp_2"), a.X+105, a.Y+y+33, true)
			f.gauge(image.Rect(a.X+129, a.Y+y+19, a.X+193, a.Y+y+27), stats.HP, stats.MaxHP, 0xf800)
			f.gauge(image.Rect(a.X+129, a.Y+y+34, a.X+193, a.Y+y+42), uint32(stats.SP), uint32(stats.MaxSP), 0x07ff)
			f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find(fmt.Sprintf("Icon_Element_%d_1", stats.Element)), a.X+205, a.Y+y+38, true)
		}
	}
}

// Companion rows use the native seven-pixel labels and six-pixel digits.
func (f *Form) smallLevel(x, y int, level byte) {
	f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Word_Lv_1"), x, y, true)
	digits := f.Env.Pics.Find("Pet_Num_Lv_Black_1")
	width, height := f.Env.Pics.Size(digits)
	height /= 10
	for i, digit := range strconv.Itoa(int(level)) {
		row := int(digit-'0') * height
		f.Env.Pics.DrawRect(f.Env.Screen, digits, x+13+i*width, y, image.Rect(0, row, width, row+height), true)
	}
}
func (f *Form) smallVitals(x, y int, s *world.Stats) {
	for i, kind := range []string{"Hp", "Sp"} {
		top := y + i*7
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Word_"+kind+"_1"), x, top, true)
		f.Env.Pics.Draw(f.Env.Screen, f.Env.Pics.Find("Pet_Frame_"+kind), x+13, top, true)
		value, maximum := s.HP, s.MaxHP
		if i == 1 {
			value, maximum = uint32(s.SP), uint32(s.MaxSP)
		}
		if maximum > 0 {
			bar := f.Env.Pics.Find("Pet_Bmp_" + kind)
			width, height := f.Env.Pics.Size(bar)
			width = int(uint64(width) * uint64(min(value, maximum)) / uint64(maximum))
			f.Env.Pics.DrawRect(f.Env.Screen, bar, x+14, top+1, image.Rect(0, 0, width, height), true)
		}
	}
}
func (f *Form) gauge(r image.Rectangle, value, maximum uint32, color uint16) {
	f.Env.Screen.Fill(r, 0x4208)
	inner := r.Inset(1)
	f.Env.Screen.Fill(inner, 0x3186)
	if maximum > 0 {
		inner.Max.X = inner.Min.X + int(uint64(inner.Dx())*uint64(min(value, maximum))/uint64(maximum))
		f.Env.Screen.Fill(inner, color)
	}
}

// Retain the native shaded confirmation panel and explicit Confirm/Cancel controls.
type confirmation struct {
	seui.Form
	owner                       *Form
	background                  *surface.Surface
	slot                        byte
	cancel                      func()
	previewWidth, previewHeight int
	previewTemplate             world.NPCTemplate
}

func (f *Form) Confirm(message string, slot byte, confirm func(), cancel ...func()) {
	if f.prompt != nil {
		f.prompt.Hide()
	}
	d := &confirmation{owner: f, slot: slot}
	if len(cancel) > 0 {
		d.cancel = cancel[0]
	}
	d.InitForm(d, f.Env)
	d.Dockable = false
	width, height := 280, 200
	if slot != 0 && f.PetPreviewSize != nil {
		d.previewWidth, d.previewHeight = f.PetPreviewSize(slot)
		if f.PetPreviewTemplate != nil {
			d.previewTemplate = f.PetPreviewTemplate(slot)
		}
		if d.previewHeight > 0 {
			// FUN_0034ba20: one text row, body extent, then the button row.
			width = max(170, len(message)*8+110+d.previewWidth+confirmationBodyMargin)
			height = max(81, d.previewHeight+confirmationBodyPadding) + confirmationButtonRow
			if d.previewTemplate.HeightPreset == 1 {
				height += confirmationLowNamePadding
			}
		}
	}
	left := 255
	if slot != 0 {
		left = (800 - width) / 2
	}
	d.Init("panel22", left, 168, 324, 0, 0, true, height, width, 30)
	d.SetMargins(50, 32, 32, 60)
	d.Image = -1 // The cached panel is the sole confirmation background.
	label := seui.NewEditor(f.Env, d)
	textX, textWidth := 80, 185
	if slot != 0 {
		textX, textWidth = 50, width-d.previewWidth-2*confirmationBodyMargin
	}
	label.Init("", textX, 0, 0, 0, 0, false, 60, textWidth, 30)
	label.ReadOnly = true
	label.SetColor(0xffe0)
	label.SetText([]byte(message))
	label.DrawText = func(x, y int, _ []byte) {
		f.Env.Text.Draw(x, label.Abs().Y+7, 0, false, true, f.Env.Screen, label.Text, 15, textWidth, 0, 0xffe0, 0)
	}
	for i, asset := range []string{"btn_ok_1", "btn_cancel_1"} {
		ok := i == 0
		b := seui.NewFixedButton(f.Env, d)
		buttonX, buttonY := 70+i*96, 150
		if slot != 0 {
			buttonX, buttonY = width/2-76+i*96, height-confirmationButtonsUp
		}
		b.Init(asset, buttonX, 20, 56, 0, 0, true, 20, 56, buttonY)
		b.OnClick = func() {
			d.Hide()
			if ok {
				confirm()
			} else if d.cancel != nil {
				d.cancel()
			}
		}
	}
	f.prompt = d
	f.Env.UI.Add(d)
	d.Show()
	f.Env.UI.SetModal(true, d)
}
func (d *confirmation) Hide() {
	if d.Env.UI.Modal == d {
		d.Env.UI.SetModal(false, d)
	}
	d.Form.Hide()
	if d.background != nil {
		d.background.Close()
		d.background = nil
	}
	d.Env.UI.Remove(d)
	if d.owner.prompt == d {
		d.owner.prompt = nil
	}
}
func (d *confirmation) KeyDown(key uint16, _ byte) {
	if key == 0x1b {
		d.Hide()
		if d.cancel != nil {
			d.cancel()
		}
	}
}
func (d *confirmation) Paint() {
	if d.background == nil {
		d.background = seui.ConfirmationBackground(d.Env, d.Width, d.Height)
	}
	a := d.Abs()
	d.Env.Screen.Draw(a.X, a.Y, d.background, true)
	if d.slot != 0 && d.owner.DrawPetBody != nil {
		x := a.X + d.Width - d.previewWidth/2 - confirmationBodyMargin
		y := a.Y + d.previewHeight + confirmationBodyTop
		if d.previewHeight == 0 {
			y = a.Y + 110
		}
		nameY := y + confirmationNameGap
		if d.previewTemplate.TalkLow == 1 {
			y = a.Y + confirmationLowBodyHeight + confirmationBodyTop
		}
		if d.previewTemplate.HeightPreset == 1 {
			nameY = a.Y + confirmationLowBodyHeight + confirmationBodyTop + confirmationNameGap
		}
		d.owner.DrawPetBody(d.slot, x, y+d.previewTemplate.SpriteDrop(), PetConfirmationAction)
		name := d.owner.Inventory.State.Pets[d.slot-1].Name
		d.Env.Text.Draw(x-len(name)*4, nameY, 0, false, true, d.Env.Screen, name, 15, 100, 0, 0xffff, 0)
	}
}
