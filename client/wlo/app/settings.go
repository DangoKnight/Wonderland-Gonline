package app

import (
	"math"
	"wonderland-go/client/wlo/hud"
	"wonderland-go/client/wlo/settings"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/internal/protocol"
)

const (
	mainSettingsButton    = 6
	escapeKey             = 0x1b
	volumeAttenuationStep = 300 // native FUN_002875b8: hundredths of a dB per step
)

func (c *Client) initSettings(path string) {
	if path == "" {
		path = c.Assets.UserPath("settings.json")
	}
	c.settingsPath = path
	c.SettingsState = settings.NewState()
	if l, err := settings.Load(path); err == nil {
		c.SettingsState.Local = l
	} else {
		c.Chat.Notice("Could not load settings: " + err.Error())
	}
	c.Settings = settings.NewForm(c.Env, c.SettingsState)
	c.UI.Add(c.Settings)
	c.Settings.Send = func(p []byte) {
		if err := c.Net.Send(p); err != nil {
			c.Chat.Notice(err.Error())
		}
	}
	c.Settings.Changed = func() {
		c.applyLocalSettings()
		if err := c.SettingsState.Local.Save(c.settingsPath); err != nil {
			c.Chat.Notice("Could not save settings: " + err.Error())
		}
	}
	c.Settings.OnHide = c.closeSettingsPrompt
	c.Settings.Action = c.settingsAction
	c.Settings.Notice = c.Chat.Notice
	c.MainButtons.Buttons[mainSettingsButton].OnClick = func() {
		if c.Settings.Visible {
			c.Settings.Hide()
		} else if c.Inventory.CanAct() {
			c.Settings.Show()
		}
	}
	c.applyLocalSettings()
}
func (c *Client) SettingsKey(key uint16) bool {
	if key != escapeKey {
		return false
	}
	if c.settingsPromptForm != nil {
		c.closeSettingsPrompt()
		return true
	}
	return c.Settings != nil && c.Settings.Escape()
}
func (c *Client) settingsPacket(p []byte) {
	if c.SettingsState.Apply(p) {
		return
	}
	// Native AC33:1 reports rejected settings; refresh from the authoritative server.
	if len(p) == 3 && p[1] == protocol.SettingsError {
		c.Chat.Notice("The server rejected that setting.")
		c.Settings.Send([]byte{protocol.CommandSettings, protocol.SettingsSnapshot})
		return
	}
	if c.Unhandled != nil {
		c.Unhandled(p)
	}
}
func audioGain(on bool, steps int) float64 {
	if !on {
		return 0
	}
	steps = max(0, min(settings.VolumeSteps, steps))
	return math.Pow(10, -float64((settings.VolumeSteps-steps)*volumeAttenuationStep)/hundredthsPerBel)
}
func (c *Client) effectsGain() float64 {
	if c.SettingsState == nil {
		return 1
	}
	l := c.SettingsState.Local
	return audioGain(l.SoundOn, l.SoundVolume)
}
func (c *Client) applyLocalSettings() {
	if c.SettingsState == nil {
		return
	}
	l := c.SettingsState.Local
	if c.Music != nil {
		c.Music.SetVolume(audioGain(l.MusicOn, l.MusicVolume))
	}
	if c.sfx != nil {
		c.sfx.SetVolume(c.effectsGain())
	}
	// Apply sound volume to active positional loops without restarting them.
	gain := c.effectsGain()
	if gain == 0 {
		c.stopAmbience()
	} else if c.World != nil {
		for _, ch := range c.ambient.ch {
			if ch != nil {
				ch.player.SetVolume(fromHundredths(ambientFalloff*abs(c.World.Player.Y-ch.srcY)) * gain)
			}
		}
	}
	if c.World != nil {
		c.World.HidePeerNames = !l.Info[settings.InfoOtherNames]
		c.World.HideOwnNickname = !l.Info[settings.InfoOwnNickname]
		c.World.HidePeerNicknames = !l.Info[settings.InfoOtherNicknames]
	}
	channels := [settings.ChannelCount]int{hud.ChannelLocal, hud.ChannelWhisper, hud.ChannelTeam, hud.ChannelGuild, hud.ChannelWorld}
	c.Chat.Colors = map[int]uint16{}
	for i, ch := range channels {
		c.Chat.Colors[ch] = settings.Palette[l.Colors[i]]
	}
	for i := range c.Chat.Lines {
		line := &c.Chat.Lines[i]
		if ink, ok := c.Chat.Colors[line.Channel]; ok {
			line.Ink = ink
		}
	}
}
func (c *Client) settingsAction(action string) {
	switch action {
	case "chat":
		c.Chat.Visible = !c.Chat.Visible
	case "readme":
		c.Chat.Notice("Wonderland Gonline: see docs/CLIENT.md and docs/GETTING_STARTED.md.")
	case "spawn":
		c.settingsPrompt("Select return point", []string{"Beach", "Record", "Carnie"}, func(i int) {
			c.Settings.Send([]byte{protocol.CommandCharacterState, protocol.CharacterStateTeleport, byte(i + 1)})
		})
	case "logout":
		c.settingsPrompt("Return to server selection?", []string{"Yes", "Cancel"}, func(i int) {
			if i == 0 {
				c.Net.Close()
				c.Settings.Hide()
				c.lostPrev()
			}
		})
	case "exit":
		c.settingsPrompt("Want to exit?", []string{"Yes", "Cancel"}, func(i int) {
			if i == 0 {
				c.Net.Close()
				c.Exit = true
			}
		})
	default:
		c.Chat.Notice("This settings service is not yet available in the Go client.")
	}
}

// settingsPrompt uses the same message panel/buttons as the native message form.
func (c *Client) settingsPrompt(message string, choices []string, selectChoice func(int)) {
	c.closeSettingsPrompt()
	f := seui.NewForm(c.Env)
	c.settingsPromptForm = f
	f.Dockable = false
	f.Init("panel15", 250, 50, 50, 0, 0, true, 140, 300, 180)
	f.SetMargins(15, 15, 15, 15)
	text := seui.NewEditor(c.Env, f)
	text.Init("", 15, 0, 0, 0, 0, false, 25, 270, 30)
	text.ReadOnly = true
	text.SetText([]byte(message))
	text.Color = 0xffff
	c.UI.Add(f)
	for i, label := range choices {
		choice := i
		b := seui.NewButton(c.Env, f)
		b.Init("btn_module_1", 15+i*90, 20, 80, 0, 0, true, 20, 80, 90)
		b.Color = 0xffff
		b.SetCaption([]byte(label))
		b.OnClick = func() { c.closeSettingsPrompt(); selectChoice(choice) }
	}
	c.UI.Modal = f
	f.Show()
}

func (c *Client) closeSettingsPrompt() {
	f := c.settingsPromptForm
	if f == nil {
		return
	}
	c.settingsPromptForm = nil
	if c.UI.Modal == f {
		c.UI.Modal = nil
	}
	f.Hide()
	c.UI.Remove(f)
}
