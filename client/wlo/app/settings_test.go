package app

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"
	"wonderland-go/client/wlo/hud"
	"wonderland-go/client/wlo/settings"
	"wonderland-go/client/wlo/seui"
)

func TestSettingsNativeMenuAndDialogs(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	readWire := wire(t, c)
	seen := 0
	sent := func() [][]byte { all := readWire(); fresh := all[seen:]; seen = len(all); return fresh }
	c.MainButtons.Buttons[mainSettingsButton].OnClick()
	if !c.Settings.Visible {
		t.Fatal("Settings menu did not open")
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{33, 2}) {
		t.Fatal(got)
	}
	c.dispatch([]byte{33, 2, 1, 1, 1, 1, 31, 0})
	c.Settings.Toggle(2)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{33, 2, 2}) {
		t.Fatal("Joining Battle", got)
	}
	if !c.SettingsState.Permissions.JoinAllowed {
		t.Fatal("request changed permission before reply")
	}
	c.dispatch([]byte{33, 2, 1, 2, 1, 1, 31, 0})
	if c.SettingsState.Permissions.JoinAllowed {
		t.Fatal("reply not adopted")
	}
	c.Settings.OpenChannels()
	c.Settings.Channels.Children[0].Base().OnClick()
	c.Settings.ConfirmChannels()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{33, 3, 30}) {
		t.Fatal("channel mask", got)
	}
	if c.SettingsState.Permissions.Channels != 31 {
		t.Fatal("channel request changed authoritative state")
	}
	c.Settings.OpenInfo()
	c.Settings.Info.Children[settings.InfoOtherNames].Base().OnClick()
	c.Settings.ConfirmInfo()
	if !c.World.HidePeerNames {
		t.Fatal("visibility not applied")
	}
	l, err := settings.Load(c.settingsPath)
	if err != nil || l.Info[settings.InfoOtherNames] {
		t.Fatal("local state not saved", err)
	}
	c.Settings.BlackName.SetText([]byte("Other"))
	c.Settings.AddBlacklist()
	c.Settings.BlackName.SetText([]byte("other"))
	c.Settings.AddBlacklist()
	if len(c.SettingsState.Local.Blacklist) != 1 {
		t.Fatal("duplicate blacklist entry")
	}
	c.Settings.BlackNames.Selected = 0
	c.Settings.DeleteBlacklist()
	if len(c.SettingsState.Local.Blacklist) != 0 {
		t.Fatal("delete")
	}
	// Check all reference forms and button assets are present before rendering.
	for _, root := range []seui.Control{c.Settings, c.Settings.Channels, c.Settings.Info, c.Settings.Titles, c.Settings.Blacklist} {
		if root.Base().Image < 0 {
			t.Fatal("missing form asset", root.Base().Name)
		}
		for _, child := range root.Base().Children {
			if b, ok := child.(*seui.FixedButton); ok && b.Image < 0 {
				t.Fatal("missing button asset", b.Left, b.Top)
			}
		}
	}
	c.Frame()
	if dir := os.Getenv("SETTINGS_SNAPSHOT"); dir != "" {
		savePNG(t, filepath.Join(dir, "settings.png"), c)
		for name, d := range map[string]*settings.Dialog{"channels": c.Settings.Channels, "info": c.Settings.Info, "titles": c.Settings.Titles, "blacklist": c.Settings.Blacklist} {
			d.Show()
			c.Frame()
			savePNG(t, filepath.Join(dir, name+".png"), c)
			d.Hide()
		}
	}
	c.Settings.OpenChannels()
	if !c.SettingsKey(27) || c.Settings.Channels.Visible || !c.Settings.Visible {
		t.Fatal("Escape child")
	}
	if !c.SettingsKey(27) || c.Settings.Visible {
		t.Fatal("Escape parent")
	}
	c.Settings.Show()
	c.dispatch(selfPacket(10002, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Other"))
	if c.Settings.Visible || c.SettingsState.Synced || c.SettingsState.Local.Info[settings.InfoOtherNames] {
		t.Fatal("character switch")
	}
}
func TestSettingsChatAndAudio(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.SettingsState.Local.Blacklist = []string{"Other"}
	c.rememberPlayer(123, []byte("Other"))
	n := len(c.Chat.Lines)
	c.dispatch([]byte{2, 3, 123, 0, 0, 0, 'h', 'i'})
	if len(c.Chat.Lines) != n {
		t.Fatal("blocked player chat displayed")
	}
	c.Settings.Color(0, 1)
	c.Chat.Say(10001, nil, []byte("Hello"), hud.ChannelLocal)
	if c.Chat.Lines[len(c.Chat.Lines)-1].Ink != settings.Palette[10] {
		t.Fatal("chat color")
	}
	c.Settings.Volume(0, -99)
	if c.SettingsState.Local.MusicVolume != 0 {
		t.Fatal("volume minimum")
	}
	c.Settings.Volume(0, 99)
	if c.SettingsState.Local.MusicVolume != 10 {
		t.Fatal("volume maximum")
	}
	if audioGain(false, 10) != 0 || audioGain(true, 10) != 1 || math.Abs(audioGain(true, 0)-math.Pow(10, -1.5)) > 1e-12 {
		t.Fatal("native volume attenuation")
	}
	c.Music = &Music{}
	c.sfx = &Sounds{}
	c.SettingsState.Local.MusicOn = false
	c.SettingsState.Local.SoundOn = false
	c.applyLocalSettings()
	if c.Music.volume != 0 || c.sfx.volume != 0 || c.effectsGain() != 0 {
		t.Fatal("audio mute")
	}
}

func TestSettingsPromptCleanupAndReturnRequest(t *testing.T) {
	c, _, movement := enteredClient(t)
	c.mapReady = true
	var sent [][]byte
	c.Settings.Send = func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }
	c.Settings.Show()
	c.settingsAction("spawn")
	prompt := c.settingsPromptForm
	if prompt == nil || c.UI.Modal != prompt {
		t.Fatal("return dialog")
	}
	c.GroundClick(300, 350)
	c.WalkKeys(true, false, false, false)
	c.groundHeld = true
	c.GroundHold(true, 300, 350)
	if len(*movement) != 0 || c.groundHeld {
		t.Fatal("settings prompt allowed world input")
	}
	// Editor, then Beach/Record/Carnie buttons.
	prompt.Children[2].Base().OnClick()
	if c.settingsPromptForm != nil || c.UI.Modal != nil || !bytes.Equal(sent[len(sent)-1], []byte{5, 17, 2}) {
		t.Fatal("record selection", sent)
	}
	c.settingsAction("logout")
	if !c.SettingsKey(27) || c.settingsPromptForm != nil || c.UI.Modal != nil || !c.Settings.Visible {
		t.Fatal("Escape prompt")
	}
	c.settingsAction("exit")
	c.Settings.Hide()
	if c.settingsPromptForm != nil || c.UI.Modal != nil || c.Exit {
		t.Fatal("parent hide left modal prompt or exited")
	}
	c.Settings.Show()
	c.settingsAction("exit")
	c.disconnected()
	if c.settingsPromptForm != nil || c.UI.Modal != nil {
		t.Fatal("disconnect left modal prompt")
	}
}
