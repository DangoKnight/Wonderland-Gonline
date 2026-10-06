package settings

import (
	"image"
	"strings"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/protocol"
)

const (
	FormWidth      = 388
	FormHeight     = 451
	SystemLeft     = (800 - FormWidth) / 2
	SystemTop      = (600 - FormHeight) / 2
	toggleWidth    = 39
	buttonHeight   = 20
	colorSampleX   = 144
	colorSampleY   = 265
	colorRowHeight = 30
)

// Form is TSe_SystemForm (FUN_00282990). Labels and borders come from the
// native white skin; controls use the constructor's original coordinates.
type Form struct {
	seui.Form
	State                             *State
	OnHide                            func()
	Send                              func([]byte)
	Changed                           func()
	Action                            func(string)
	Notice                            func(string)
	Toggles                           map[byte]*seui.FixedButton
	Music, Sound, Zoom                *seui.FixedButton
	Channels, Info, Titles, Blacklist *Dialog
	channelDraft                      byte
	infoDraft                         [InfoCount]bool
	channelButtons                    [ChannelCount]*seui.FixedButton
	infoButtons                       [InfoCount]*seui.FixedButton
	BlackName                         *seui.Editor
	BlackNames                        *seui.SelectText
}

func button(env *seui.Env, owner seui.Control, asset string, x, y, w, h int, click func()) *seui.FixedButton {
	b := seui.NewFixedButton(env, owner)
	b.Init(asset, x, h, w, 0, 0, true, h, w, y)
	b.OnClick = click
	b.ClickSound = true
	b.TabStop = false
	return b
}
func panel(env *seui.Env, name string, x, y, w, h int) *Dialog {
	d := &Dialog{}
	d.InitForm(d, env)
	d.Dockable = false
	d.SetMargins(0, 0, 0, 0)
	d.Init(name, x, h, w, 0, 0, true, h, w, y)
	env.UI.Add(d)
	return d
}

// Dialog adds no labels to the baked native artwork.
type Dialog struct{ seui.Form }

func NewForm(env *seui.Env, state *State) *Form {
	f := &Form{State: state, Toggles: map[byte]*seui.FixedButton{}}
	f.InitForm(f, env)
	f.Dockable = false
	f.SetMargins(0, 0, 0, 0)
	f.Init("Form_System_2", SystemLeft, FormHeight, FormWidth, 0, 0, true, FormHeight, FormWidth, SystemTop)
	button(env, f, "Btn_Close_s_1", 338, 24, 18, 18, f.Hide)
	button(env, f, "Btn_Close_1", 166, 412, 56, 20, f.Hide)
	for _, row := range []struct {
		option byte
		y      int
	}{{protocol.SettingsPK, 57}, {protocol.SettingsSnapshot, 83}, {protocol.SettingsTrade, 109}, {protocol.SettingsPartyInvites, 212}} {
		option := row.option
		f.Toggles[option] = button(env, f, "Btn_On_1", 143, row.y, toggleWidth, buttonHeight, func() { f.Toggle(option) })
	}
	f.Music = button(env, f, "Btn_On_1", 289, 84, toggleWidth, buttonHeight, func() { state.Local.MusicOn = !state.Local.MusicOn; f.changed() })
	f.Sound = button(env, f, "Btn_On_1", 250, 154, toggleWidth, buttonHeight, func() { state.Local.SoundOn = !state.Local.SoundOn; f.changed() })
	f.Zoom = button(env, f, "Btn_On_1", 143, 185, toggleWidth, buttonHeight, func() {
		if f.Notice != nil {
			f.Notice("Zoom Mode rendering is not yet available.")
		}
	})
	for i := range 2 {
		row := i
		y := 115 + 70*i
		button(env, f, "Btn_ArrowL_4", 237, y, 17, 17, func() { f.Volume(row, -1) })
		button(env, f, "Btn_ArrowR_4", 334, y, 17, 17, func() { f.Volume(row, 1) })
	}
	for i := range ChannelCount {
		row := i
		y := 266 + colorRowHeight*i
		button(env, f, "Btn_ArrowL_5", 124, y, 17, 17, func() { f.Color(row, -1) })
		button(env, f, "Btn_ArrowR_5", 183, y, 17, 17, func() { f.Color(row, 1) })
	}
	f.initDialogs()
	button(env, f, "Btn_setup_1", 143, 134, 39, 20, f.OpenChannels)
	button(env, f, "Btn_setup_1", 143, 160, 39, 20, f.OpenInfo)
	button(env, f, "Btn_setup_1", 143, 237, 39, 20, func() { f.open(f.Titles) })
	button(env, f, "Btn_setup_1", 295, 233, 39, 20, func() { f.refreshBlacklist(); f.open(f.Blacklist) })
	for _, b := range []struct {
		asset, action string
		x, y          int
	}{
		{"Btn_ExchangeLockSet", "security", 218, 206}, {"Btn_SetAccount", "login", 298, 206},
		{"Btn_OfficialWeb_1", "website", 213, 263}, {"Btn_ChangePassword_1", "password", 295, 263},
		{"Btn_Readme_1", "readme", 213, 288}, {"Btn_ReTurn_1", "spawn", 295, 288},
		{"Btn_SetMsgForm", "chat", 213, 313}, {"Btn_CheckNumber_1", "balance", 295, 313},
		{"Btn_Exchange_2", "redeem", 213, 338}, {"Btn_Vip_1", "vip", 295, 338},
		{"btn_ReFill_1", "mycard", 213, 363}, {"Btn_Logout_1", "logout", 295, 363},
		{"Btn_NewebPay_1", "points", 213, 388}, {"Btn_Exit_2", "exit", 295, 388},
	} {
		action := b.action
		button(env, f, b.asset, b.x, b.y, 71, 20, func() {
			if f.Action != nil {
				f.Action(action)
			}
		})
	}
	return f
}

func (f *Form) send(p []byte) {
	if f.Send != nil {
		f.Send(p)
	}
}
func (f *Form) changed() {
	if f.Changed != nil {
		f.Changed()
	}
}
func (f *Form) Show() {
	f.Form.Show()
	f.send([]byte{protocol.CommandSettings, protocol.SettingsSnapshot})
}
func (f *Form) Hide() {
	f.closeDialogs()
	if f.OnHide != nil {
		f.OnHide()
	}
	f.Form.Hide()
}
func (f *Form) closeDialogs() {
	for _, d := range []*Dialog{f.Channels, f.Info, f.Titles, f.Blacklist} {
		if d != nil {
			d.Hide()
		}
	}
}
func (f *Form) open(d *Dialog) { f.closeDialogs(); d.Show() }

// Escape closes the foremost settings dialog before the parent window.
func (f *Form) Escape() bool {
	for _, d := range []*Dialog{f.Channels, f.Info, f.Titles, f.Blacklist} {
		if d.Visible {
			d.Hide()
			return true
		}
	}
	if !f.Visible {
		return false
	}
	f.Hide()
	return true
}
func (f *Form) Toggle(option byte) {
	if !f.State.Synced {
		return
	}
	s := f.State.Permissions
	var on bool
	switch option {
	case protocol.SettingsPK:
		on = s.PKAllowed
	case protocol.SettingsJoinBattle:
		on = s.JoinAllowed
	case protocol.SettingsTrade:
		on = s.TradeAllowed
	case protocol.SettingsPartyInvites:
		on = !s.PartyInvitesBlocked
	default:
		return
	}
	f.send([]byte{protocol.CommandSettings, option, Flag(!on)})
}
func (f *Form) Volume(row, delta int) {
	v := &f.State.Local.MusicVolume
	if row == 1 {
		v = &f.State.Local.SoundVolume
	}
	*v = max(0, min(VolumeSteps, *v+delta))
	f.changed()
}
func (f *Form) Color(row, delta int) {
	if row < 0 || row >= ChannelCount {
		return
	}
	v := &f.State.Local.Colors[row]
	*v = (*v + delta + len(Palette)) % len(Palette)
	f.changed()
}
func setToggle(env *seui.Env, b *seui.FixedButton, on bool) {
	asset := "Btn_Off_1"
	if on {
		asset = "Btn_On_1"
	}
	b.Image = env.Pics.Find(asset)
}
func (f *Form) Paint() {
	s := f.State.Permissions
	for option, b := range f.Toggles {
		b.Enabled = f.State.Synced
		on := s.PKAllowed
		switch option {
		case protocol.SettingsJoinBattle:
			on = s.JoinAllowed
		case protocol.SettingsTrade:
			on = s.TradeAllowed
		case protocol.SettingsPartyInvites:
			on = !s.PartyInvitesBlocked
		}
		setToggle(f.Env, b, on)
	}
	setToggle(f.Env, f.Music, f.State.Local.MusicOn)
	setToggle(f.Env, f.Sound, f.State.Local.SoundOn)
	setToggle(f.Env, f.Zoom, f.State.Local.Zoom)
	f.Panel.Paint()
	for row, label := range []string{"Local", "Whisper", "Team", "Guild", "World"} {
		f.Env.Text.Draw(f.Left+colorSampleX, f.Top+colorSampleY+row*colorRowHeight, 0, false, true, f.Env.Screen, []byte(label), 0, 60, 0, Palette[f.State.Local.Colors[row]], 2)
	}
	for row, v := range []int{f.State.Local.MusicVolume, f.State.Local.SoundVolume} {
		// FUN_002870ac fills Panel30 proportionally across the baked volume bar.
		r := image.Rect(f.Left+255, f.Top+120+row*70, f.Left+333, f.Top+127+row*70)
		f.Env.Screen.Fill(r, 0x2106)
		r.Max.X = r.Min.X + r.Dx()*v/VolumeSteps
		f.Env.Screen.Fill(r, 0x9cef)
	}
}

func (f *Form) initDialogs() {
	env := f.Env
	// TTH_channelForm, FUN_00281fcc; edits are drafts until Confirm.
	f.Channels = panel(env, "Form_Channel_1", 47, 97, 169, 237)
	for i := range ChannelCount {
		row := i
		f.channelButtons[i] = button(env, f.Channels, "Btn_On_1", 86, 47+30*i, 39, 20, func() { f.channelDraft ^= 1 << row; f.refreshChannels() })
	}
	button(env, f.Channels, "Btn_OK_2", 41, 199, 39, 20, f.ConfirmChannels)
	button(env, f.Channels, "Btn_Close_2", 86, 199, 39, 20, f.Channels.Hide)
	// TTH_HuminfoForm, FUN_002824bc.
	f.Info = panel(env, "Form_HumInfo_1", 150, 105, 186, 379)
	for i := range InfoCount {
		row := i
		f.infoButtons[i] = button(env, f.Info, "btn_Check_1", 19, 43+30*i, 16, 16, func() { f.infoDraft[row] = !f.infoDraft[row]; f.refreshInfo() })
	}
	button(env, f.Info, "Btn_OK_2", 51, 337, 39, 20, f.ConfirmInfo)
	button(env, f.Info, "Btn_Close_2", 97, 337, 39, 20, f.Info.Hide)
	// TSe_PlayerTitleForm, FUN_002a4b40: five rows are empty until awarded.
	f.Titles = panel(env, "Form_PlayerTitle", 80, 93, 250, 275)
	button(env, f.Titles, "Btn_Close_s_1", 200, 26, 18, 18, f.Titles.Hide)
	button(env, f.Titles, "Btn_Close_2", 105, 246, 39, 20, f.Titles.Hide)
	// The title entitlement/list receive path is not ported; never fabricate awards.
	// THL_BlackNameListForm, FUN_0024ecb8 (supplement in reference units).
	f.Blacklist = panel(env, "form_BlackNameList", 55, 41, 264, 415)
	button(env, f.Blacklist, "Btn_Close_s_1", 215, 24, 18, 18, f.Blacklist.Hide)
	button(env, f.Blacklist, "Btn_Close_1", 105, 380, 56, 20, f.Blacklist.Hide)
	f.BlackName = seui.NewEditor(env, f.Blacklist)
	f.BlackName.Init("", 99, 0, 0, 0, 0, false, 20, 86, 59)
	f.BlackName.MaxLen = 14
	f.BlackName.Limit = true
	f.BlackName.Color = 0
	f.BlackName.OnEnter = f.AddBlacklist
	button(env, f.Blacklist, "Btn_OK_2", 197, 59, 39, 20, f.AddBlacklist)
	f.BlackNames = seui.NewSelectText(env, f.Blacklist)
	f.BlackNames.RowHeight = 26
	f.BlackNames.Spacing = 3
	f.BlackNames.TextX = 10
	f.BlackNames.TextY = 1
	f.BlackNames.ShowIcons = false
	f.BlackNames.Embossed = false
	f.BlackNames.FocusBox = false
	f.BlackNames.SetBounds(26, 90, 240, 190)
	f.BlackNames.ShowSel = true
	f.BlackNames.Color = 0xffff
	scroll := seui.NewScrollButton(env, f.Blacklist)
	scroll.Init("", 217, 182, 17, 0, 0, true, 245, 17, 105)
	scroll.Visible_ = f.BlackNames.RowsShown
	scroll.SetThumb("bar_H4", 0, 21, 9, 0)
	scroll.SetCaps(7, 16)
	scroll.SetVertical(true)
	scroll.OnChange = func() { f.BlackNames.TopIndex = scroll.Pos }
	f.BlackNames.Scroll = scroll
	button(env, f.Blacklist, "Btn_ArrowUp_1", 217, 88, 17, 17, scroll.StepUp)
	button(env, f.Blacklist, "Btn_ArrowDn_1", 217, 349, 17, 17, scroll.StepDown)
	button(env, f.Blacklist, "Btn_Delete_1", 111, 348, 43, 20, f.DeleteBlacklist)
}
func (f *Form) OpenChannels() {
	f.channelDraft = f.State.Permissions.Channels
	f.refreshChannels()
	f.open(f.Channels)
}
func (f *Form) refreshChannels() {
	for i, b := range f.channelButtons {
		setToggle(f.Env, b, f.channelDraft&(1<<i) != 0)
	}
}
func (f *Form) ConfirmChannels() {
	if !f.State.Synced {
		return
	}
	f.send([]byte{protocol.CommandSettings, protocol.SettingsChannels, f.channelDraft})
	f.Channels.Hide()
}
func (f *Form) OpenInfo() { f.infoDraft = f.State.Local.Info; f.refreshInfo(); f.open(f.Info) }
func (f *Form) refreshInfo() {
	for i, b := range f.infoButtons {
		asset := "btn_UnCheck_1"
		if f.infoDraft[i] {
			asset = "btn_Check_1"
		}
		b.Image = f.Env.Pics.Find(asset)
	}
}
func (f *Form) ConfirmInfo() { f.State.Local.Info = f.infoDraft; f.changed(); f.Info.Hide() }
func (f *Form) AddBlacklist() {
	name := strings.TrimSpace(DecodeName(f.BlackName.Text))
	if name == "" || f.State.Local.Blocked(clientassets.Big5Text(name)) {
		return
	}
	f.State.Local.Blacklist = append(f.State.Local.Blacklist, name)
	f.BlackName.Clear()
	f.refreshBlacklist()
	f.changed()
}
func (f *Form) DeleteBlacklist() {
	i := f.BlackNames.Selected
	if i < 0 || i >= len(f.State.Local.Blacklist) {
		return
	}
	f.State.Local.Blacklist = append(f.State.Local.Blacklist[:i], f.State.Local.Blacklist[i+1:]...)
	f.refreshBlacklist()
	f.changed()
}
func (f *Form) refreshBlacklist() {
	f.BlackNames.Clear()
	for _, n := range f.State.Local.Blacklist {
		f.BlackNames.Add(clientassets.Big5Text(n), "", "")
	}
	f.BlackNames.Selected = -1
	f.BlackNames.TopIndex = 0
}
